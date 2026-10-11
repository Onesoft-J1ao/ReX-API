package controller

import (
	"context"
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type bangumiTorrentTestFixture struct {
	name, hash      string
	body, sanitized []byte
	files           []bangumiTorrentFile
}

func bangumiTorrentTestString(value string) string { return strconv.Itoa(len(value)) + ":" + value }

func bangumiTorrentFixture(name string, files []bangumiTorrentFile) bangumiTorrentTestFixture {
	var info strings.Builder
	info.WriteString("d5:filesl")
	var total int64
	for _, file := range files {
		info.WriteString("d6:lengthi" + strconv.FormatInt(file.Length, 10) + "e4:pathl")
		for part := range strings.SplitSeq(file.Name, "/") {
			info.WriteString(bangumiTorrentTestString(part))
		}
		info.WriteString("ee")
		total += file.Length
	}
	info.WriteString("e4:name" + bangumiTorrentTestString(name) + "12:piece lengthi16384e6:pieces")
	pieces := int((total-1)/16384 + 1)
	if pieces < 1 {
		pieces = 1
	}
	info.WriteString(bangumiTorrentTestString(strings.Repeat("p", pieces*20)) + "e")
	rawInfo := info.String()
	digest := sha1.Sum([]byte(rawInfo))
	fixture := bangumiTorrentTestFixture{name: name, hash: hex.EncodeToString(digest[:]), sanitized: []byte("d4:info" + rawInfo + "e")}
	fixture.body = []byte("d8:announce" + bangumiTorrentTestString("http://127.0.0.1/private-tracker") + "4:info" + rawInfo + "8:url-list" + bangumiTorrentTestString("http://169.254.169.254/webseed") + "e")
	fixture.files = append([]bangumiTorrentFile{}, files...)
	for i := range fixture.files {
		fixture.files[i].Name = name + "/" + fixture.files[i].Name
	}
	return fixture
}

type bangumiTorrentBackendFixture struct {
	t                                 *testing.T
	server                            *httptest.Server
	mu                                sync.Mutex
	fixtures                          map[string]bangumiTorrentTestFixture
	active                            map[int]bangumiTorrentDetail
	nextID                            int
	lists, adds                       int
	updates                           [][]int
	deleted                           []int
	deleteAttempts                    int
	addBodies                         [][]byte
	folders                           []string
	failAdd, missingID, failDelete    bool
	failUpdateBefore, failUpdateAfter bool
	addStarted                        chan struct{}
	allowAdd                          chan struct{}
}

func newBangumiTorrentBackendFixture(t *testing.T, fixtures ...bangumiTorrentTestFixture) *bangumiTorrentBackendFixture {
	t.Helper()
	backend := &bangumiTorrentBackendFixture{t: t, fixtures: make(map[string]bangumiTorrentTestFixture), active: make(map[int]bangumiTorrentDetail)}
	for _, fixture := range fixtures {
		backend.fixtures[fixture.hash] = fixture
	}
	backend.server = httptest.NewServer(http.HandlerFunc(backend.serve))
	t.Cleanup(backend.server.Close)
	return backend
}

func (backend *bangumiTorrentBackendFixture) send(w http.ResponseWriter, value any) {
	body, err := common.Marshal(value)
	if !assert.NoError(backend.t, err) {
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (backend *bangumiTorrentBackendFixture) serve(w http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/torrents" && request.Method == http.MethodGet {
		backend.mu.Lock()
		list := make([]bangumiTorrentDetail, 0, len(backend.active))
		for _, detail := range backend.active {
			detail.Files = slices.Clone(detail.Files)
			list = append(list, detail)
		}
		backend.mu.Unlock()
		backend.send(w, map[string]any{"torrents": list})
		return
	}
	if request.URL.Path == "/torrents" && request.Method == http.MethodPost {
		body, err := io.ReadAll(request.Body)
		if !assert.NoError(backend.t, err) {
			w.WriteHeader(500)
			return
		}
		backend.mu.Lock()
		var fixture bangumiTorrentTestFixture
		if request.URL.Query().Get("is_url") == "true" {
			hash := strings.TrimPrefix(string(body), "magnet:?xt=urn:btih:")
			fixture = backend.fixtures[hash]
			assert.Equal(backend.t, "magnet:?xt=urn:btih:"+fixture.hash, string(body))
		} else {
			for _, candidate := range backend.fixtures {
				if string(candidate.sanitized) == string(body) {
					fixture = candidate
					break
				}
			}
		}
		if !assert.NotEmpty(backend.t, fixture.hash, "backend must receive canonical metadata, not a remote website URL or unsafe tracker fields") {
			backend.mu.Unlock()
			w.WriteHeader(400)
			return
		}
		files := slices.Clone(fixture.files)
		if request.URL.Query().Get("list_only") == "true" {
			backend.lists++
			assert.Empty(backend.t, request.URL.Query().Get("only_files"))
			for i := range files {
				files[i].Included = true
			}
			backend.mu.Unlock()
			backend.send(w, map[string]any{"details": bangumiTorrentDetail{InfoHash: fixture.hash, Name: fixture.name, Files: files}})
			return
		}
		assert.Equal(backend.t, "false", request.URL.Query().Get("overwrite"))
		selected, err := strconv.Atoi(request.URL.Query().Get("only_files"))
		if !assert.NoError(backend.t, err) || !assert.GreaterOrEqual(backend.t, selected, 0) || !assert.Less(backend.t, selected, len(files)) {
			backend.mu.Unlock()
			w.WriteHeader(400)
			return
		}
		for i := range files {
			files[i].Included = i == selected
		}
		id := backend.nextID
		backend.nextID++
		folder := request.URL.Query().Get("output_folder")
		detail := bangumiTorrentDetail{ID: &id, InfoHash: fixture.hash, Name: fixture.name, Files: files, OutputFolder: folder}
		backend.active[id] = detail
		backend.adds++
		backend.addBodies = append(backend.addBodies, append([]byte{}, body...))
		backend.folders = append(backend.folders, folder)
		failAdd, missingID, started, gate := backend.failAdd, backend.missingID, backend.addStarted, backend.allowAdd
		backend.mu.Unlock()
		// No BitTorrent client runs: one tiny placeholder makes output cleanup
		// observable at the same HTTP/filesystem boundary as a real backend.
		filename := filepath.Join(folder, filepath.FromSlash(files[selected].Name))
		assert.NoError(backend.t, os.MkdirAll(filepath.Dir(filename), 0700))
		assert.NoError(backend.t, os.WriteFile(filename, []byte("fixture"), 0600))
		if started != nil {
			started <- struct{}{}
		}
		if gate != nil {
			select {
			case <-gate:
			case <-request.Context().Done():
				return
			}
		}
		if failAdd {
			w.WriteHeader(500)
			return
		}
		if missingID {
			detail.ID = nil
			backend.send(w, map[string]any{"id": nil, "details": detail})
			return
		}
		backend.send(w, map[string]any{"id": id, "details": detail})
		return
	}
	var id int
	if _, err := fmt.Sscanf(request.URL.Path, "/torrents/%d", &id); err == nil {
		backend.mu.Lock()
		detail, exists := backend.active[id]
		if !exists {
			backend.mu.Unlock()
			w.WriteHeader(404)
			return
		}
		if strings.HasSuffix(request.URL.Path, "/delete") && request.Method == http.MethodPost {
			backend.deleteAttempts++
			if backend.failDelete {
				backend.mu.Unlock()
				w.WriteHeader(500)
				return
			}
			delete(backend.active, id)
			backend.deleted = append(backend.deleted, id)
			backend.mu.Unlock()
			backend.send(w, map[string]any{})
			return
		}
		if strings.HasSuffix(request.URL.Path, "/update_only_files") && request.Method == http.MethodPost {
			var change struct {
				OnlyFiles []int `json:"only_files"`
			}
			if err := common.DecodeJson(request.Body, &change); !assert.NoError(backend.t, err) {
				backend.mu.Unlock()
				w.WriteHeader(400)
				return
			}
			assert.NotEmpty(backend.t, change.OnlyFiles)
			if backend.failUpdateBefore {
				backend.failUpdateBefore = false
				backend.mu.Unlock()
				w.WriteHeader(500)
				return
			}
			detail.Files = slices.Clone(detail.Files)
			for i := range detail.Files {
				detail.Files[i].Included = slices.Contains(change.OnlyFiles, i)
			}
			backend.active[id] = detail
			backend.updates = append(backend.updates, slices.Clone(change.OnlyFiles))
			failAfter := backend.failUpdateAfter
			backend.failUpdateAfter = false
			backend.mu.Unlock()
			if failAfter {
				for _, index := range change.OnlyFiles {
					filename := filepath.Join(detail.OutputFolder, filepath.FromSlash(detail.Files[index].Name))
					assert.NoError(backend.t, os.MkdirAll(filepath.Dir(filename), 0700))
					assert.NoError(backend.t, os.WriteFile(filename, []byte("partially downloaded fixture"), 0600))
				}
				w.WriteHeader(500)
				return
			}
			backend.send(w, map[string]any{})
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == fmt.Sprintf("/torrents/%d", id) {
			detail.Files = slices.Clone(detail.Files)
			backend.mu.Unlock()
			backend.send(w, detail)
			return
		}
		backend.mu.Unlock()
	}
	backend.t.Errorf("unexpected backend request %s %s; prepare must not open a stream or forget a torrent", request.Method, request.URL.Path)
	w.WriteHeader(400)
}

func (backend *bangumiTorrentBackendFixture) snapshot() (int, int, [][]int, []int, []string) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	updates := make([][]int, len(backend.updates))
	for i := range updates {
		updates[i] = slices.Clone(backend.updates[i])
	}
	return backend.lists, backend.adds, updates, slices.Clone(backend.deleted), slices.Clone(backend.folders)
}

func newBangumiTorrentResources(t *testing.T, backend *bangumiTorrentBackendFixture, budget int64, fixtures ...bangumiTorrentTestFixture) (*bangumiTorrentManager, map[string]string, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		for _, fixture := range fixtures {
			if request.URL.Path == "/"+fixture.hash+".torrent" {
				w.Header().Set("Content-Type", "application/x-bittorrent")
				_, _ = w.Write(fixture.body)
				return
			}
		}
		t.Errorf("unrecognized source metadata request")
		w.WriteHeader(404)
	}))
	t.Cleanup(source.Close)
	manager, err := newBangumiTorrentManager(backend.server.URL, filepath.Join(t.TempDir(), "owned"), source.Client(), budget)
	require.NoError(t, err)
	clock := &atomic.Int64{}
	clock.Store(time.Now().UnixNano())
	manager.now = func() time.Time { return time.Unix(0, clock.Load()) }
	locations := make(map[string]string)
	for _, fixture := range fixtures {
		locations[fixture.hash] = source.URL + "/" + fixture.hash + ".torrent"
	}
	return manager, locations, clock
}

func bangumiTorrentResolved(source, uri string) service.BangumiSourceResolveResult {
	return service.BangumiSourceResolveResult{BangumiSourceStatus: service.BangumiSourceStatus{SourceID: source, State: service.BangumiSourceReady}, Format: "torrent", TorrentURI: uri}
}

func TestBangumiSourceTorrentExactFileSelection(t *testing.T) {
	fixture := bangumiTorrentFixture("Pack", []bangumiTorrentFile{{Name: "01.mp4", Length: 40}, {Name: "02.mkv", Length: 80}, {Name: "SP 特别篇.mp4", Length: 20}, {Name: "字幕.ass", Length: 5}})
	backend := newBangumiTorrentBackendFixture(t, fixture)
	manager, locations, _ := newBangumiTorrentResources(t, backend, 1000, fixture)
	resolved := bangumiTorrentResolved("source-a", locations[fixture.hash])
	selection, err := manager.prepare(context.Background(), resolved, "episode-original", "")
	require.NoError(t, err)
	assert.Equal(t, "selection_required", selection.State)
	require.Len(t, selection.Files, 3)
	assert.Equal(t, "Pack/SP 特别篇.mp4", selection.Files[2].Name)
	assert.EqualValues(t, 80, selection.Files[1].Size)
	assert.NotEqual(t, "1", selection.Files[1].ID)
	lists, adds, _, _, _ := backend.snapshot()
	assert.Equal(t, 1, lists)
	assert.Zero(t, adds, "metadata inspection must not start downloading a season pack")
	for _, tc := range []struct{ name, source, episode, file string }{
		{"other source", "source-b", "episode-original", selection.Files[0].ID},
		{"other episode", "source-a", "episode-other", selection.Files[0].ID},
		{"fileId is not episodeId", "source-a", selection.Files[0].ID, selection.Files[0].ID},
		{"guessed index", "source-a", "episode-original", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := manager.prepare(context.Background(), bangumiTorrentResolved(tc.source, locations[fixture.hash]), tc.episode, tc.file)
			require.Error(t, err)
			assert.Equal(t, service.BangumiSourceFailed, result.State)
			assert.Empty(t, result.target)
		})
	}
	prepared, err := manager.prepare(context.Background(), resolved, "episode-original", selection.Files[0].ID)
	require.NoError(t, err)
	assert.Equal(t, service.BangumiSourceReady, prepared.State)
	assert.Equal(t, backend.server.URL+"/torrents/0/stream/0", prepared.target, "file zero must be chosen explicitly, not replaced by the largest file")
	encoded, err := common.Marshal(prepared)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), backend.server.URL)
	assert.NotContains(t, string(encoded), locations[fixture.hash])
}

func TestBangumiSourceTorrentSingleVideoAndNoFallback(t *testing.T) {
	cases := []struct {
		name  string
		files []bangumiTorrentFile
		ready bool
		index int
	}{
		{"single video among subtitles", []bangumiTorrentFile{{Name: "subtitles.ass", Length: 10}, {Name: "episode.mp4", Length: 40}}, true, 1},
		{"no video", []bangumiTorrentFile{{Name: "large.dat", Length: 80}, {Name: "subtitles.ass", Length: 10}}, false, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := bangumiTorrentFixture(tc.name, tc.files)
			backend := newBangumiTorrentBackendFixture(t, fixture)
			manager, locations, _ := newBangumiTorrentResources(t, backend, 1000, fixture)
			result, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[fixture.hash]), "not-a-file-number", "")
			if tc.ready {
				require.NoError(t, err)
				assert.Equal(t, backend.server.URL+fmt.Sprintf("/torrents/0/stream/%d", tc.index), result.target)
			} else {
				require.Error(t, err)
				assert.Empty(t, result.target)
			}
			_, adds, _, _, _ := backend.snapshot()
			if tc.ready {
				assert.Equal(t, 1, adds)
			} else {
				assert.Zero(t, adds)
			}
		})
	}
}

func TestBangumiSourceTorrentSelectionIdentityAndExpiry(t *testing.T) {
	first := bangumiTorrentFixture("First", []bangumiTorrentFile{{Name: "01.mp4", Length: 10}, {Name: "02.mp4", Length: 10}})
	second := bangumiTorrentFixture("Second", []bangumiTorrentFile{{Name: "01.mp4", Length: 10}, {Name: "02.mp4", Length: 10}})
	backend := newBangumiTorrentBackendFixture(t, first, second)
	manager, locations, clock := newBangumiTorrentResources(t, backend, 1000, first, second)
	selection, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[first.hash]), "episode", "")
	require.NoError(t, err)
	result, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[second.hash]), "episode", selection.Files[0].ID)
	require.Error(t, err)
	assert.Empty(t, result.target)
	clock.Add(int64(11 * time.Minute))
	result, err = manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[first.hash]), "episode", selection.Files[0].ID)
	require.Error(t, err)
	assert.Empty(t, result.target)
	_, adds, _, _, _ := backend.snapshot()
	assert.Zero(t, adds)
}

func TestBangumiSourceTorrentAdmissionBudgets(t *testing.T) {
	oversize := bangumiTorrentFixture("TooLarge", []bangumiTorrentFile{{Name: "episode.mp4", Length: 101}})
	first := bangumiTorrentFixture("One", []bangumiTorrentFile{{Name: "episode.mp4", Length: 60}})
	second := bangumiTorrentFixture("Two", []bangumiTorrentFile{{Name: "episode.mp4", Length: 60}})
	backend := newBangumiTorrentBackendFixture(t, oversize, first, second)
	manager, locations, _ := newBangumiTorrentResources(t, backend, 100, oversize, first, second)
	result, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[oversize.hash]), "episode", "")
	require.Error(t, err)
	assert.Empty(t, result.target)
	_, adds, _, _, _ := backend.snapshot()
	assert.Zero(t, adds)
	_, err = manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[first.hash]), "episode", "")
	require.NoError(t, err)
	result, err = manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[second.hash]), "episode", "")
	require.Error(t, err)
	assert.Empty(t, result.target)
	_, adds, _, _, _ = backend.snapshot()
	assert.Equal(t, 1, adds, "cumulative budget must be charged before contacting add")
	t.Run("two managed torrents", func(t *testing.T) {
		third := bangumiTorrentFixture("Three", []bangumiTorrentFile{{Name: "episode.mp4", Length: 10}})
		otherBackend := newBangumiTorrentBackendFixture(t, first, second, third)
		other, urls, _ := newBangumiTorrentResources(t, otherBackend, 1000, first, second, third)
		for _, fixture := range []bangumiTorrentTestFixture{first, second} {
			_, err := other.prepare(context.Background(), bangumiTorrentResolved("source", urls[fixture.hash]), "episode", "")
			require.NoError(t, err)
		}
		result, err := other.prepare(context.Background(), bangumiTorrentResolved("source", urls[third.hash]), "episode", "")
		require.Error(t, err)
		assert.Empty(t, result.target)
		_, adds, _, _, _ := otherBackend.snapshot()
		assert.Equal(t, 2, adds)
	})
}

func TestBangumiSourceTorrentConcurrentFirstAdd(t *testing.T) {
	fixture := bangumiTorrentFixture("Concurrent", []bangumiTorrentFile{{Name: "episode.mp4", Length: 40}})
	backend := newBangumiTorrentBackendFixture(t, fixture)
	backend.addStarted, backend.allowAdd = make(chan struct{}, 2), make(chan struct{})
	manager, locations, _ := newBangumiTorrentResources(t, backend, 1000, fixture)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan error, 2)
	go func() {
		_, err := manager.prepare(ctx, bangumiTorrentResolved("source", locations[fixture.hash]), "episode", "")
		results <- err
	}()
	select {
	case <-backend.addStarted:
	case <-ctx.Done():
		close(backend.allowAdd)
		t.Fatal("first add did not arrive")
	}
	go func() {
		_, err := manager.prepare(ctx, bangumiTorrentResolved("source", locations[fixture.hash]), "episode", "")
		results <- err
	}()
	close(backend.allowAdd)
	for range 2 {
		require.NoError(t, <-results)
	}
	lists, adds, _, _, _ := backend.snapshot()
	assert.Equal(t, 1, lists)
	assert.Equal(t, 1, adds)
}

func TestBangumiSourceTorrentActiveFileUnionAndIdleCleanup(t *testing.T) {
	fixture := bangumiTorrentFixture("Season", []bangumiTorrentFile{{Name: "01.mp4", Length: 40}, {Name: "02.mp4", Length: 60}})
	backend := newBangumiTorrentBackendFixture(t, fixture)
	manager, locations, clock := newBangumiTorrentResources(t, backend, 1000, fixture)
	resolved := bangumiTorrentResolved("source", locations[fixture.hash])
	selection, err := manager.prepare(context.Background(), resolved, "episode", "")
	require.NoError(t, err)
	first, err := manager.prepare(context.Background(), resolved, "episode", selection.Files[0].ID)
	require.NoError(t, err)
	finishFirst, err := first.onStart()
	require.NoError(t, err)
	finishAnotherRange, err := first.onStart()
	require.NoError(t, err)
	second, err := manager.prepare(context.Background(), resolved, "episode", selection.Files[1].ID)
	require.NoError(t, err)
	finishSecond, err := second.onStart()
	require.NoError(t, err)
	_, _, updates, _, _ := backend.snapshot()
	require.NotEmpty(t, updates)
	assert.Equal(t, []int{0, 1}, updates[len(updates)-1], "changing episode must not cancel existing readers")
	clock.Add(int64(2 * time.Hour))
	require.NoError(t, manager.sweep(context.Background(), manager.now()))
	_, _, _, deleted, _ := backend.snapshot()
	assert.Empty(t, deleted)
	finishFirst()
	finishFirst()
	finishAnotherRange()
	_, _, updates, _, _ = backend.snapshot()
	assert.Equal(t, []int{1}, updates[len(updates)-1])
	clock.Add(int64(time.Hour))
	require.NoError(t, manager.sweep(context.Background(), manager.now()))
	_, _, _, deleted, _ = backend.snapshot()
	assert.Empty(t, deleted, "second file is still being streamed")
	finishSecond()
	clock.Add(int64(19 * time.Minute))
	require.NoError(t, manager.sweep(context.Background(), manager.now()))
	_, _, _, deleted, _ = backend.snapshot()
	assert.Empty(t, deleted, "idle time starts at the final stream's completion")
	clock.Add(int64(2 * time.Minute))
	require.NoError(t, manager.sweep(context.Background(), manager.now()))
	_, _, _, deleted, folders := backend.snapshot()
	assert.Equal(t, []int{0}, deleted)
	_, err = os.Stat(folders[0])
	assert.True(t, os.IsNotExist(err), "delete must remove owned retained data, not forget the torrent")
	_, err = first.onStart()
	require.Error(t, err, "an evicted grant must never resolve to another torrent/file")
}

func TestBangumiSourceTorrentStreamCompletionContract(t *testing.T) {
	for _, mode := range []string{"client disconnect", "write failure", "HEAD"} {
		t.Run(mode, func(t *testing.T) {
			fixture := bangumiTorrentFixture("Flow", []bangumiTorrentFile{{Name: "episode.mp4", Length: 40}})
			backend := newBangumiTorrentBackendFixture(t, fixture)
			manager, locations, clock := newBangumiTorrentResources(t, backend, 1000, fixture)
			prepared, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[fixture.hash]), "episode", "")
			require.NoError(t, err)
			started, released := make(chan struct{}), make(chan struct{})
			writeError := make(chan error, 1)
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				finish, err := prepared.onStart()
				if !assert.NoError(t, err) {
					w.WriteHeader(500)
					return
				}
				defer func() { finish(); close(released) }()
				close(started)
				switch mode {
				case "client disconnect":
					w.WriteHeader(206)
					w.(http.Flusher).Flush()
					<-request.Context().Done()
				case "write failure":
					connection, _, err := w.(http.Hijacker).Hijack()
					if !assert.NoError(t, err) {
						return
					}
					_ = connection.Close()
					_, err = w.Write([]byte("must fail"))
					writeError <- err
				case "HEAD":
					w.WriteHeader(200)
					return
				}
			}))
			defer receiver.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			method := http.MethodGet
			if mode == "HEAD" {
				method = http.MethodHead
			}
			request, err := http.NewRequestWithContext(ctx, method, receiver.URL, nil)
			require.NoError(t, err)
			request.Header.Set("Range", "bytes=0-10")
			response, requestErr := receiver.Client().Do(request)
			if response != nil {
				defer response.Body.Close()
			}
			if mode != "write failure" {
				require.NoError(t, requestErr)
			}
			<-started
			if mode == "client disconnect" {
				cancel()
			}
			select {
			case <-released:
			case <-time.After(5 * time.Second):
				t.Fatal("receiver defer did not release torrent reference")
			}
			if mode == "write failure" {
				require.Error(t, <-writeError)
			}
			clock.Add(int64(21 * time.Minute))
			require.NoError(t, manager.sweep(context.Background(), manager.now()))
			_, _, _, deleted, _ := backend.snapshot()
			assert.Equal(t, []int{0}, deleted, "onStart recipient must defer release across every HTTP completion path")
		})
	}
}

func TestBangumiSourceTorrentFailedFileUpdateRollback(t *testing.T) {
	for _, mode := range []string{"no partial data", "partial data with reader", "update without reader"} {
		t.Run(mode, func(t *testing.T) {
			pack := bangumiTorrentFixture("RollbackPack", []bangumiTorrentFile{{Name: "01.mp4", Length: 40}, {Name: "02.mp4", Length: 80}})
			other := bangumiTorrentFixture("Other", []bangumiTorrentFile{{Name: "episode.mp4", Length: 100}})
			backend := newBangumiTorrentBackendFixture(t, pack, other)
			manager, locations, _ := newBangumiTorrentResources(t, backend, 200, pack, other)
			resolved := bangumiTorrentResolved("source", locations[pack.hash])
			selection, err := manager.prepare(context.Background(), resolved, "episode", "")
			require.NoError(t, err)
			first, err := manager.prepare(context.Background(), resolved, "episode", selection.Files[0].ID)
			require.NoError(t, err)
			var finish func()
			if mode != "update without reader" {
				finish, err = first.onStart()
				require.NoError(t, err)
			}
			backend.mu.Lock()
			backend.failUpdateBefore = mode == "no partial data"
			backend.failUpdateAfter = mode != "no partial data"
			backend.mu.Unlock()
			failed, err := manager.prepare(context.Background(), resolved, "episode", selection.Files[1].ID)
			require.Error(t, err)
			assert.Empty(t, failed.target)
			_, err = first.onStart()
			require.Error(t, err, "faulted resource must reject new Range/HEAD references")
			if mode == "partial data with reader" {
				_, err := manager.prepare(context.Background(), bangumiTorrentResolved("other", locations[other.hash]), "episode", "")
				require.Error(t, err, "retained partial data is not a fictitious free budget")
				_, _, _, deleted, _ := backend.snapshot()
				assert.Empty(t, deleted, "active reader must not be canceled to hide failed admission")
			} else {
				_, err := manager.prepare(context.Background(), bangumiTorrentResolved("other", locations[other.hash]), "episode", "")
				require.NoError(t, err, "verified unused reservation must be returned")
			}
			if finish != nil {
				finish()
			}
			_, _, _, deleted, _ := backend.snapshot()
			assert.Contains(t, deleted, 0, "last reference drains the faulted resource immediately")
			_, err = manager.prepare(context.Background(), bangumiTorrentResolved("other", locations[other.hash]), "episode", "")
			require.NoError(t, err)
		})
	}
}

func TestBangumiSourceTorrentCleanupRetryIsBounded(t *testing.T) {
	failed := bangumiTorrentFixture("Retry", []bangumiTorrentFile{{Name: "episode.mp4", Length: 80}})
	fresh := bangumiTorrentFixture("FreshAfterRestart", []bangumiTorrentFile{{Name: "episode.mp4", Length: 50}})
	backend := newBangumiTorrentBackendFixture(t, failed, fresh)
	backend.failAdd, backend.failDelete = true, true
	manager, locations, clock := newBangumiTorrentResources(t, backend, 100, failed, fresh)
	_, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[failed.hash]), "episode", "")
	require.Error(t, err)
	for _, step := range []struct {
		advance   time.Duration
		attempts  int
		wantError bool
	}{{10 * time.Second, 1, false}, {31 * time.Second, 2, true}, {61 * time.Second, 3, true}, {121 * time.Second, 4, true}} {
		clock.Add(int64(step.advance))
		err := manager.sweep(context.Background(), manager.now())
		if step.wantError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		backend.mu.Lock()
		attempts := backend.deleteAttempts
		backend.mu.Unlock()
		assert.Equal(t, step.attempts, attempts)
	}
	clock.Add(int64(24 * time.Hour))
	require.NoError(t, manager.sweep(context.Background(), manager.now()))
	backend.mu.Lock()
	attempts := backend.deleteAttempts
	backend.failAdd, backend.failDelete = false, false
	backend.mu.Unlock()
	assert.Equal(t, 4, attempts, "parked cleanup must not keep issuing connections")
	restarted, err := newBangumiTorrentManager(backend.server.URL, manager.root, manager.publicClient, 100)
	require.NoError(t, err)
	_, err = restarted.prepare(context.Background(), bangumiTorrentResolved("source", locations[fresh.hash]), "episode", "")
	require.NoError(t, err, "restart must recheck its persisted pending cleanup")
	_, _, _, deleted, _ := backend.snapshot()
	assert.Contains(t, deleted, 0)
}

func TestBangumiSourceTorrentFailedAdmissionCleanup(t *testing.T) {
	first := bangumiTorrentFixture("Failure", []bangumiTorrentFile{{Name: "episode.mp4", Length: 80}})
	second := bangumiTorrentFixture("AfterRollback", []bangumiTorrentFile{{Name: "episode.mp4", Length: 50}})
	for _, mode := range []string{"add lost response", "missing target", "delete failure holds budget"} {
		t.Run(mode, func(t *testing.T) {
			backend := newBangumiTorrentBackendFixture(t, first, second)
			backend.failAdd = mode != "missing target"
			backend.missingID = mode == "missing target"
			backend.failDelete = mode == "delete failure holds budget"
			manager, locations, clock := newBangumiTorrentResources(t, backend, 100, first, second)
			result, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[first.hash]), "episode", "")
			require.Error(t, err)
			assert.Equal(t, service.BangumiSourceFailed, result.State)
			assert.Empty(t, result.target)
			backend.mu.Lock()
			backend.failAdd, backend.missingID = false, false
			backend.mu.Unlock()
			if mode == "delete failure holds budget" {
				result, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[second.hash]), "episode", "")
				require.Error(t, err)
				assert.Empty(t, result.target)
				backend.mu.Lock()
				backend.failDelete = false
				backend.mu.Unlock()
				clock.Add(int64(time.Minute))
				require.NoError(t, manager.sweep(context.Background(), manager.now()))
			}
			result, err = manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[second.hash]), "episode", "")
			require.NoError(t, err, "confirmed cleanup must roll back the prior reservation")
			assert.Equal(t, service.BangumiSourceReady, result.State)
			_, _, _, deleted, folders := backend.snapshot()
			assert.Contains(t, deleted, 0)
			_, err = os.Stat(folders[0])
			assert.True(t, os.IsNotExist(err))
		})
	}
}

func TestBangumiSourceTorrentRestartOwnership(t *testing.T) {
	owned := bangumiTorrentFixture("Owned", []bangumiTorrentFile{{Name: "episode.mp4", Length: 40}})
	orphan := bangumiTorrentFixture("Orphan", []bangumiTorrentFile{{Name: "episode.mp4", Length: 40}})
	fresh := bangumiTorrentFixture("Fresh", []bangumiTorrentFile{{Name: "episode.mp4", Length: 40}})
	backend := newBangumiTorrentBackendFixture(t, owned, orphan, fresh)
	manager, locations, _ := newBangumiTorrentResources(t, backend, 1000, owned, orphan, fresh)
	_, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[owned.hash]), "episode", "")
	require.NoError(t, err)
	orphanFolder := filepath.Join(manager.root, orphan.hash)
	require.NoError(t, os.MkdirAll(orphanFolder, 0700))
	require.NoError(t, bangumiTorrentWriteJSON(filepath.Join(orphanFolder, ".rex-owner.json"), bangumiTorrentDirectoryOwner{Owner: manager.owner, Hash: orphan.hash}))
	foreignFolder := filepath.Join(t.TempDir(), "other-client")
	require.NoError(t, os.MkdirAll(foreignFolder, 0700))
	foreignFile := filepath.Join(foreignFolder, "keep.txt")
	require.NoError(t, os.WriteFile(foreignFile, []byte("other client's data"), 0600))
	orphanID, foreignID := 42, 77
	backend.mu.Lock()
	backend.active[orphanID] = bangumiTorrentDetail{ID: &orphanID, InfoHash: orphan.hash, OutputFolder: orphanFolder, Files: orphan.files}
	backend.active[foreignID] = bangumiTorrentDetail{ID: &foreignID, InfoHash: strings.Repeat("a", 40), OutputFolder: foreignFolder}
	backend.mu.Unlock()
	restarted, err := newBangumiTorrentManager(backend.server.URL, manager.root, manager.publicClient, 1000)
	require.NoError(t, err)
	_, err = restarted.prepare(context.Background(), bangumiTorrentResolved("source", locations[fresh.hash]), "episode", "")
	require.NoError(t, err)
	_, _, _, deleted, _ := backend.snapshot()
	assert.ElementsMatch(t, []int{0, 42}, deleted)
	body, err := os.ReadFile(foreignFile)
	require.NoError(t, err)
	assert.Equal(t, "other client's data", string(body))
	backend.mu.Lock()
	_, exists := backend.active[foreignID]
	backend.mu.Unlock()
	assert.True(t, exists)
}

func TestBangumiSourceTorrentNeverClaimsUnknownOutput(t *testing.T) {
	fixture := bangumiTorrentFixture("Unclaimed", []bangumiTorrentFile{{Name: "episode.mp4", Length: 40}})
	backend := newBangumiTorrentBackendFixture(t, fixture)
	manager, locations, _ := newBangumiTorrentResources(t, backend, 1000, fixture)
	unknownFolder := filepath.Join(manager.root, fixture.hash)
	require.NoError(t, os.MkdirAll(unknownFolder, 0700))
	unknownFile := filepath.Join(unknownFolder, "other-client.txt")
	require.NoError(t, os.WriteFile(unknownFile, []byte("do not claim or remove"), 0600))
	result, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[fixture.hash]), "episode", "")
	require.Error(t, err)
	assert.Empty(t, result.target)
	_, adds, _, deleted, _ := backend.snapshot()
	assert.Zero(t, adds)
	assert.Empty(t, deleted)
	body, err := os.ReadFile(unknownFile)
	require.NoError(t, err)
	assert.Equal(t, "do not claim or remove", string(body))
}

func TestBangumiSourceTorrentUncreatedIntentDoesNotOwnDirectory(t *testing.T) {
	fixture := bangumiTorrentFixture("Intent", []bangumiTorrentFile{{Name: "episode.mp4", Length: 40}})
	backend := newBangumiTorrentBackendFixture(t, fixture)
	manager, locations, _ := newBangumiTorrentResources(t, backend, 1000, fixture)
	folder := filepath.Join(manager.root, fixture.hash)
	require.NoError(t, os.MkdirAll(folder, 0700))
	foreign := filepath.Join(folder, "keep.txt")
	require.NoError(t, os.WriteFile(foreign, []byte("unproven directory"), 0600))
	require.NoError(t, bangumiTorrentWriteJSON(filepath.Join(manager.root, ".rex-managed.json"), bangumiTorrentJournal{Version: 1, Owner: manager.owner, Entries: []*bangumiTorrentOwned{{Hash: fixture.hash, DirectoryOwned: false, Retained: map[int]int64{0: 40}}}}))
	restarted, err := newBangumiTorrentManager(backend.server.URL, manager.root, manager.publicClient, 1000)
	require.NoError(t, err)
	result, err := restarted.prepare(context.Background(), bangumiTorrentResolved("source", locations[fixture.hash]), "episode", "")
	require.Error(t, err)
	assert.Empty(t, result.target)
	body, err := os.ReadFile(foreign)
	require.NoError(t, err)
	assert.Equal(t, "unproven directory", string(body))
	_, adds, _, deleted, _ := backend.snapshot()
	assert.Zero(t, adds)
	assert.Empty(t, deleted)
}

func TestBangumiSourceTorrentNeverAdoptsForeignTorrent(t *testing.T) {
	fixture := bangumiTorrentFixture("SharedHash", []bangumiTorrentFile{{Name: "episode.mp4", Length: 40}})
	backend := newBangumiTorrentBackendFixture(t, fixture)
	manager, locations, _ := newBangumiTorrentResources(t, backend, 1000, fixture)
	id := 77
	backend.active[id] = bangumiTorrentDetail{ID: &id, InfoHash: fixture.hash, OutputFolder: t.TempDir(), Files: fixture.files}
	result, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", locations[fixture.hash]), "episode", "")
	require.Error(t, err)
	assert.Empty(t, result.target)
	_, adds, _, deleted, _ := backend.snapshot()
	assert.Zero(t, adds)
	assert.Empty(t, deleted)
}

func TestBangumiSourceTorrentBackendIDReuseFailsClosed(t *testing.T) {
	fixture := bangumiTorrentFixture("Original", []bangumiTorrentFile{{Name: "episode.mp4", Length: 40}})
	backend := newBangumiTorrentBackendFixture(t, fixture)
	manager, locations, _ := newBangumiTorrentResources(t, backend, 1000, fixture)
	resolved := bangumiTorrentResolved("source", locations[fixture.hash])
	prepared, err := manager.prepare(context.Background(), resolved, "episode", "")
	require.NoError(t, err)
	id := 0
	backend.mu.Lock()
	backend.active[id] = bangumiTorrentDetail{ID: &id, InfoHash: strings.Repeat("a", 40), OutputFolder: t.TempDir(), Files: fixture.files}
	backend.mu.Unlock()
	_, err = prepared.onStart()
	require.Error(t, err, "stale grant must not stream an unrelated reused ID")
	result, err := manager.prepare(context.Background(), resolved, "episode", "")
	require.NoError(t, err, "fresh resolve may create a new owned resource")
	assert.Equal(t, backend.server.URL+"/torrents/1/stream/0", result.target)
	_, err = prepared.onStart()
	require.Error(t, err, "old grant callback must not validate a new resource while still pointing at reused ID zero")
	_, _, updates, deleted, _ := backend.snapshot()
	assert.Empty(t, updates)
	assert.Empty(t, deleted)
}

func TestBangumiSourceTorrentMagnetAndMetadataBoundaries(t *testing.T) {
	fixture := bangumiTorrentFixture("Magnet", []bangumiTorrentFile{{Name: "episode.mp4", Length: 40}})
	for _, name := range []string{"hex removes URL hints", "base32", "invalid BTIH", "ambiguous BTIH", "missing URI", "unsafe metainfo path"} {
		t.Run(name, func(t *testing.T) {
			selectedFixture := fixture
			if name == "unsafe metainfo path" {
				selectedFixture = bangumiTorrentFixture("Traversal", []bangumiTorrentFile{{Name: "../outside.mp4", Length: 40}})
			}
			backend := newBangumiTorrentBackendFixture(t, selectedFixture)
			manager, locations, _ := newBangumiTorrentResources(t, backend, 1000, selectedFixture)
			uri := "magnet:?xt=urn:btih:" + fixture.hash + "&tr=http://127.0.0.1/private&xs=http://169.254.169.254/&as=file:///etc/passwd&ws=http://localhost/"
			good := true
			switch name {
			case "base32":
				hash, err := hex.DecodeString(fixture.hash)
				require.NoError(t, err)
				uri = "magnet:?xt=urn:btih:" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hash)
			case "invalid BTIH":
				uri = "magnet:?xt=urn:btih:" + strings.Repeat("g", 40)
				good = false
			case "ambiguous BTIH":
				uri += "&xt=urn:btih:" + fixture.hash
				good = false
			case "missing URI":
				uri = ""
				good = false
			case "unsafe metainfo path":
				uri = locations[selectedFixture.hash]
				good = false
			}
			result, err := manager.prepare(context.Background(), bangumiTorrentResolved("source", uri), "episode", "")
			if good {
				require.NoError(t, err)
				assert.NotEmpty(t, result.target)
			} else {
				require.Error(t, err)
				assert.Empty(t, result.target)
			}
			_, adds, _, _, _ := backend.snapshot()
			if good {
				assert.Equal(t, 1, adds)
			} else {
				assert.Zero(t, adds)
			}
		})
	}
}
