package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	bangumiTorrentBodyLimit      = 2 << 20
	bangumiTorrentFileLimit      = 2048
	bangumiTorrentSelectionTTL   = 10 * time.Minute
	bangumiTorrentIdleTTL        = 20 * time.Minute
	bangumiTorrentControlTimeout = 20 * time.Second
)

type bangumiTorrentFile struct {
	Name     string `json:"name"`
	Length   int64  `json:"length"`
	Included bool   `json:"included"`
}

type bangumiTorrentDetail struct {
	ID           *int                 `json:"id"`
	InfoHash     string               `json:"info_hash"`
	Name         string               `json:"name"`
	OutputFolder string               `json:"output_folder"`
	Files        []bangumiTorrentFile `json:"files"`
}

type bangumiTorrentMetadata struct {
	hash    string
	body    []byte
	isURL   bool
	files   []bangumiTorrentFile
	diskNames []string
	expires time.Time
}

type bangumiTorrentMetadataWait struct {
	done     chan struct{}
	metadata *bangumiTorrentMetadata
	err      error
}

type bangumiTorrentFileChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type bangumiTorrentPreparation struct {
	SourceID  string                     `json:"sourceId"`
	State     string                     `json:"state"`
	Format    string                     `json:"format"`
	Files     []bangumiTorrentFileChoice `json:"files,omitempty"`
	Message   string                     `json:"message,omitempty"`
	StreamURL string                     `json:"streamUrl,omitempty"`
	target    string
	hash      string
	fileIndex int
	onStart   func() (func(), error)
}

type bangumiTorrentFileBinding struct {
	sourceID, episodeID, hash string
	index                     int
	expires                   time.Time
}

type bangumiTorrentOwned struct {
	Hash      string        `json:"hash"`
	ID        *int          `json:"id"`
	DirectoryOwned bool     `json:"directoryOwned"`
	CleanupPending bool `json:"cleanupPending"`
	Retained  map[int]int64 `json:"retained"`
	cleanupDeadline, nextCleanup time.Time
	cleanupAttempts int
	cleanupParked bool
	metadata  *bangumiTorrentMetadata
	wanted    int
	included  []int
	refs      map[int]int
	idleSince time.Time
}

type bangumiTorrentRootOwner struct {
	Version int    `json:"version"`
	Owner   string `json:"owner"`
}

type bangumiTorrentJournal struct {
	Version int                    `json:"version"`
	Owner   string                 `json:"owner"`
	Entries []*bangumiTorrentOwned `json:"entries"`
}

type bangumiTorrentDirectoryOwner struct {
	Owner string `json:"owner"`
	Hash  string `json:"hash"`
}

// control serializes admission, only_files changes and deletion. Active streams
// hold references, not the control lock. bytes counts every file retained on
// disk, including files excluded from a later only_files selection.
type bangumiTorrentManager struct {
	backend         *url.URL
	backendClient   *http.Client
	publicClient    *http.Client
	root, owner     string
	maxBytes, bytes int64
	control         chan struct{}
	entries         map[string]*bangumiTorrentOwned
	reconciled      bool
	startupPending bool
	now             func() time.Time
	mu              sync.Mutex
	metadata        map[string]*bangumiTorrentMetadata
	pending         map[string]*bangumiTorrentMetadataWait
	bindings        map[string]bangumiTorrentFileBinding
}

var bangumiTorrentHost struct {
	sync.Once
	manager *bangumiTorrentManager
	err     error
}

func resolveBangumiTorrentSource(c *gin.Context, resolved service.BangumiSourceResolveResult, episodeID string) {
	if resolved.SourceID == "" || len(resolved.SourceID) > 128 || episodeID == "" || len(episodeID) > 128 || resolved.Format != "torrent" || resolved.State != service.BangumiSourceReady {
		c.JSON(http.StatusOK, gin.H{"sourceId": resolved.SourceID, "state": "failed", "format": "torrent", "message": "Invalid resolved torrent source"})
		return
	}
	fileID := c.Query("fileId")
	if len(fileID) > 128 {
		c.JSON(http.StatusOK, gin.H{"sourceId": resolved.SourceID, "state": "failed", "format": "torrent", "message": "Invalid torrent file selection"})
		return
	}
	bangumiTorrentHost.Do(func() {
		client, err := service.NewBangumiPublicHTTPClient(common.BangumiProxyURL)
		if err != nil {
			bangumiTorrentHost.err = errors.New("Protected torrent source client is unavailable")
			return
		}
		backend := os.Getenv("BANGUMI_TORRENT_BACKEND_URL")
		if backend == "" {
			backend = "http://127.0.0.1:3031"
		}
		root := os.Getenv("BANGUMI_TORRENT_OUTPUT_DIR")
		if root == "" {
			cache, err := os.UserCacheDir()
			if err != nil {
				root = filepath.FromSlash("/var/lib/rex-api/bangumi-source-torrents")
			} else {
				root = filepath.Join(cache, "rex-api", "bangumi-source-torrents")
			}
		}
		// This absolute, writable path must be shared with rqbit at the same
		// location. Failure is explicit; there is no alternate output directory.
		budget := int64(3 << 30)
		if configured := os.Getenv("BANGUMI_TORRENT_MAX_BYTES"); configured != "" {
			budget, err = strconv.ParseInt(configured, 10, 64)
			if err != nil {
				bangumiTorrentHost.err = errors.New("Invalid torrent retention budget")
				return
			}
		}
		bangumiTorrentHost.manager, bangumiTorrentHost.err = newBangumiTorrentManager(backend, root, client, budget)
		if bangumiTorrentHost.err == nil {
			go func() {
				ticker := time.NewTicker(time.Minute)
				defer ticker.Stop()
				for now := range ticker.C {
					ctx, cancel := context.WithTimeout(context.Background(), bangumiTorrentControlTimeout)
					_ = bangumiTorrentHost.manager.sweep(ctx, now)
					cancel()
				}
			}()
		}
	})
	if bangumiTorrentHost.err != nil {
		c.JSON(http.StatusOK, gin.H{"sourceId": resolved.SourceID, "state": "invalid_config", "format": "torrent", "message": "Torrent backend or owned output directory is unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	prepared, err := bangumiTorrentHost.manager.prepare(ctx, resolved, episodeID, fileID)
	if err != nil {
		prepared.Message = err.Error()
		c.JSON(http.StatusOK, prepared)
		return
	}
	if prepared.State != service.BangumiSourceReady {
		c.JSON(http.StatusOK, prepared)
		return
	}
	streamURL, err := newBangumiMediaGrantWithStart(c, prepared.target, nil, prepared.onStart)
	if err != nil {
		bangumiTorrentHost.manager.abandon(prepared.hash)
		c.JSON(http.StatusOK, gin.H{"sourceId": resolved.SourceID, "state": "failed", "format": "torrent", "message": "Torrent playback could not be authorized"})
		return
	}
	prepared.StreamURL = streamURL
	c.JSON(http.StatusOK, prepared)
}

func newBangumiTorrentManager(rawBackend, root string, public *http.Client, maxBytes int64) (*bangumiTorrentManager, error) {
	backend, err := url.Parse(rawBackend)
	if err != nil || (backend.Scheme != "http" && backend.Scheme != "https") || backend.Hostname() == "" || backend.User != nil || (backend.Path != "" && backend.Path != "/") || backend.RawQuery != "" || backend.Fragment != "" {
		return nil, errors.New("Invalid trusted torrent backend URL")
	}
	if public == nil || maxBytes <= 0 || maxBytes > 1<<40 {
		return nil, errors.New("Invalid torrent client or retention budget")
	}
	root, err = filepath.Abs(root)
	if err != nil || root == filepath.VolumeName(root)+string(filepath.Separator) {
		return nil, errors.New("Invalid owned torrent output directory")
	}
	if info, err := os.Lstat(root); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("Torrent output directory must not be a symlink")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, errors.New("Cannot create owned torrent output directory")
	}
	ownerPath := filepath.Join(root, ".rex-root.json")
	var ownership bangumiTorrentRootOwner
	if _, err := os.Stat(ownerPath); os.IsNotExist(err) {
		nodes, err := os.ReadDir(root)
		if err != nil || len(nodes) != 0 {
			return nil, errors.New("Refusing to claim a nonempty torrent output directory")
		}
		ownership = bangumiTorrentRootOwner{Version: 1, Owner: rand.Text()}
		if err := bangumiTorrentWriteJSON(ownerPath, ownership); err != nil {
			return nil, err
		}
	} else if err := bangumiTorrentReadJSON(ownerPath, &ownership); err != nil {
		return nil, err
	}
	if ownership.Version != 1 || len(ownership.Owner) < 20 || len(ownership.Owner) > 128 {
		return nil, errors.New("Invalid torrent output ownership marker")
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxIdleConns: 4, MaxIdleConnsPerHost: 4}
	m := &bangumiTorrentManager{backend: backend, backendClient: &http.Client{Transport: transport, Timeout: bangumiTorrentControlTimeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("Torrent control redirects are not allowed")
	}}, publicClient: public, root: root, owner: ownership.Owner, maxBytes: maxBytes, control: make(chan struct{}, 1), entries: make(map[string]*bangumiTorrentOwned), now: time.Now, metadata: make(map[string]*bangumiTorrentMetadata), pending: make(map[string]*bangumiTorrentMetadataWait), bindings: make(map[string]bangumiTorrentFileBinding)}
	journalPath := filepath.Join(root, ".rex-managed.json")
	if _, err := os.Stat(journalPath); err == nil {
		var journal bangumiTorrentJournal
		if err := bangumiTorrentReadJSON(journalPath, &journal); err != nil {
			return nil, err
		}
		if journal.Version != 1 || journal.Owner != m.owner || len(journal.Entries) > 32 {
			return nil, errors.New("Invalid torrent ownership journal")
		}
		for _, entry := range journal.Entries {
			if entry == nil || !bangumiTorrentHash(entry.Hash) || (entry.ID != nil && *entry.ID < 0) || len(entry.Retained) > bangumiTorrentFileLimit {
				return nil, errors.New("Invalid owned torrent record")
			}
			entry.refs = make(map[int]int)
			entry.wanted = -1
			for index, size := range entry.Retained {
				if index < 0 || size <= 0 || size > (1<<40)-m.bytes {
					return nil, errors.New("Invalid persisted torrent budget")
				}
				m.bytes += size
			}
			m.entries[entry.Hash] = entry
		}
	} else if !os.IsNotExist(err) {
		return nil, errors.New("Cannot read torrent ownership journal")
	}
	// A verified per-directory marker recovers owned orphan outputs whose journal
	// update was interrupted. Unknown directories and other clients are ignored.
	nodes, err := os.ReadDir(root)
	if err != nil || len(nodes) > 256 {
		return nil, errors.New("Cannot enumerate owned torrent outputs")
	}
	for _, node := range nodes {
		if !node.IsDir() || !bangumiTorrentHash(node.Name()) {
			continue
		}
		var marker bangumiTorrentDirectoryOwner
		if err := bangumiTorrentReadJSON(filepath.Join(root, node.Name(), ".rex-owner.json"), &marker); err != nil {
			continue
		}
		if marker.Owner != m.owner || marker.Hash != node.Name() {
			continue
		}
		if _, exists := m.entries[marker.Hash]; !exists {
			m.entries[marker.Hash] = &bangumiTorrentOwned{Hash: marker.Hash, Retained: make(map[int]int64), refs: make(map[int]int), wanted: -1}
		}
	}
	return m, nil
}

func bangumiTorrentWriteJSON(filename string, value any) error {
	body, err := common.Marshal(value)
	if err != nil {
		return errors.New("Cannot encode torrent ownership state")
	}
	file, err := os.OpenFile(filename+".work", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("Cannot persist torrent ownership state")
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return errors.New("Cannot persist torrent ownership state")
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return errors.New("Cannot persist torrent ownership state")
	}
	if err := file.Close(); err != nil {
		return errors.New("Cannot persist torrent ownership state")
	}
	if err := os.Rename(filename+".work", filename); err != nil {
		return errors.New("Cannot publish torrent ownership state")
	}
	return nil
}

func bangumiTorrentReadJSON(filename string, value any) error {
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128<<10 {
		return errors.New("Invalid torrent ownership state file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return errors.New("Cannot read torrent ownership state")
	}
	defer file.Close()
	if err := common.DecodeJson(io.LimitReader(file, 128<<10), value); err != nil {
		return errors.New("Invalid torrent ownership state JSON")
	}
	return nil
}

func (m *bangumiTorrentManager) saveJournal() error {
	journal := bangumiTorrentJournal{Version: 1, Owner: m.owner, Entries: make([]*bangumiTorrentOwned, 0, len(m.entries))}
	for _, entry := range m.entries {
		journal.Entries = append(journal.Entries, entry)
	}
	return bangumiTorrentWriteJSON(filepath.Join(m.root, ".rex-managed.json"), journal)
}

func (m *bangumiTorrentManager) lock(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.New("Torrent operation canceled or expired")
	}
	select {
	case m.control <- struct{}{}:
		return nil
	case <-ctx.Done():
		return errors.New("Torrent operation canceled or expired")
	}
}
func (m *bangumiTorrentManager) unlock() { <-m.control }

func (m *bangumiTorrentManager) backendRequest(ctx context.Context, method, endpoint string, query url.Values, body []byte, value any) error {
	ctx, cancel := context.WithTimeout(ctx, bangumiTorrentControlTimeout)
	defer cancel()
	target := *m.backend
	target.Path, target.RawPath, target.RawQuery = endpoint, "", query.Encode()
	request, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return errors.New("Cannot construct torrent backend request")
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/octet-stream")
		if strings.Contains(endpoint, "update_only_files") {
			request.Header.Set("Content-Type", "application/json")
		}
	}
	response, err := m.backendClient.Do(request)
	if err != nil {
		return errors.New("Torrent backend request failed or timed out")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return bangumiTorrentNotFound
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("Torrent backend rejected the operation")
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, bangumiTorrentBodyLimit+1))
	if err != nil || len(payload) > bangumiTorrentBodyLimit {
		return errors.New("Invalid or oversized torrent backend response")
	}
	if value != nil && common.Unmarshal(payload, value) != nil {
		return errors.New("Invalid torrent backend JSON response")
	}
	return nil
}

var bangumiTorrentNotFound = errors.New("torrent no longer exists")

func (m *bangumiTorrentManager) listBackend(ctx context.Context) ([]bangumiTorrentDetail, error) {
	var response struct {
		Torrents []bangumiTorrentDetail `json:"torrents"`
	}
	if err := m.backendRequest(ctx, http.MethodGet, "/torrents", nil, nil, &response); err != nil {
		return nil, err
	}
	if response.Torrents == nil || len(response.Torrents) > 256 {
		return nil, errors.New("Invalid torrent backend registry")
	}
	return response.Torrents, nil
}

func (m *bangumiTorrentManager) detail(ctx context.Context, id int) (bangumiTorrentDetail, error) {
	var detail bangumiTorrentDetail
	err := m.backendRequest(ctx, http.MethodGet, fmt.Sprintf("/torrents/%d", id), nil, nil, &detail)
	if err == nil && (detail.ID == nil || *detail.ID != id) {
		err = errors.New("Torrent backend returned an inconsistent identity")
	}
	return detail, err
}

func (m *bangumiTorrentManager) ownedFolder(hash string) string { return filepath.Join(m.root, hash) }
func (m *bangumiTorrentManager) ownsDetail(detail bangumiTorrentDetail, hash string) bool {
	return detail.ID != nil && *detail.ID >= 0 && strings.EqualFold(detail.InfoHash, hash) && filepath.Clean(detail.OutputFolder) == m.ownedFolder(hash)
}

func (m *bangumiTorrentManager) deleteOwned(ctx context.Context, entry *bangumiTorrentOwned) error {
	folder := m.ownedFolder(entry.Hash)
	if !entry.DirectoryOwned {
		var marker bangumiTorrentDirectoryOwner
		if bangumiTorrentReadJSON(filepath.Join(folder, ".rex-owner.json"), &marker) == nil && marker.Owner == m.owner && marker.Hash == entry.Hash {
			entry.DirectoryOwned = true
		} else {
			if _, err := os.Lstat(folder); err == nil || !os.IsNotExist(err) { return errors.New("Unproven torrent output directory will not be reused or deleted") }
			// An intent recorded before exclusive directory creation is not proof
			// of ownership over another client's directory or backend torrent.
			for _, size := range entry.Retained { m.bytes -= size }
			delete(m.entries, entry.Hash)
			return m.saveJournal()
		}
	}
	torrents, err := m.listBackend(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range torrents {
		if candidate.ID == nil || *candidate.ID < 0 {
			return errors.New("Invalid backend torrent identity")
		}
		if !strings.EqualFold(candidate.InfoHash, entry.Hash) && filepath.Clean(candidate.OutputFolder) != m.ownedFolder(entry.Hash) {
			continue
		}
		detail, err := m.detail(ctx, *candidate.ID)
		if errors.Is(err, bangumiTorrentNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if filepath.Clean(detail.OutputFolder) == m.ownedFolder(entry.Hash) && !strings.EqualFold(detail.InfoHash, entry.Hash) {
			return errors.New("Owned torrent directory is occupied by an unrelated backend torrent")
		}
		if !m.ownsDetail(detail, entry.Hash) {
			continue
		}
		if err := m.backendRequest(ctx, http.MethodPost, fmt.Sprintf("/torrents/%d/delete", *detail.ID), nil, nil, nil); err != nil {
			return err
		}
		if _, err := m.detail(ctx, *detail.ID); !errors.Is(err, bangumiTorrentNotFound) { return errors.New("Owned torrent deletion could not be verified") }
	}
	if info, err := os.Lstat(folder); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("Unsafe owned torrent output directory")
	}
	if err := os.RemoveAll(folder); err != nil {
		return errors.New("Cannot remove owned torrent output files")
	}
	for _, size := range entry.Retained {
		m.bytes -= size
	}
	delete(m.entries, entry.Hash)
	return m.saveJournal()
}

func (m *bangumiTorrentManager) reconcile(ctx context.Context) error {
	if m.reconciled { return nil }
	for _, entry := range m.entries { _ = m.tryCleanup(ctx, entry, m.now(), true) }
	m.reconciled = true
	m.startupPending = len(m.entries) != 0
	if err := m.saveJournal(); err != nil { return err }
	if m.startupPending { return errors.New("Owned torrent startup cleanup is pending; no resource is admitted") }
	return nil
}

func (m *bangumiTorrentManager) markFault(entry *bangumiTorrentOwned, now time.Time) {
	entry.CleanupPending = true
	if entry.cleanupDeadline.IsZero() { entry.cleanupDeadline = now.Add(5*time.Minute) }
	_ = m.saveJournal()
}

func (m *bangumiTorrentManager) tryCleanup(ctx context.Context, entry *bangumiTorrentOwned, now time.Time, force bool) error {
	if len(entry.refs) != 0 {
		if entry.CleanupPending && !entry.cleanupParked && !entry.cleanupDeadline.IsZero() && !now.Before(entry.cleanupDeadline) {
			entry.cleanupParked = true
			common.SysError("Bangumi torrent "+entry.Hash[:12]+": active faulted resource parked for last-release or restart cleanup")
			_ = m.saveJournal()
		}
		return nil
	}
	if entry.CleanupPending {
		if entry.cleanupParked && !force { return nil }
		if entry.cleanupAttempts >= 4 || (!force && !entry.cleanupDeadline.IsZero() && !now.Before(entry.cleanupDeadline)) {
			if !entry.cleanupParked { entry.cleanupParked = true; common.SysError("Bangumi torrent "+entry.Hash[:12]+": cleanup retry limit reached; ownership retained for restart"); _ = m.saveJournal() }
			return nil
		}
		if !force && now.Before(entry.nextCleanup) { return nil }
	}
	entry.cleanupAttempts++
	if err := m.deleteOwned(ctx, entry); err != nil {
		m.markFault(entry, now)
		delay := min(30*time.Second*time.Duration(1<<min(entry.cleanupAttempts-1, 4)), 5*time.Minute)
		entry.nextCleanup = now.Add(delay)
		entry.cleanupParked = entry.cleanupAttempts >= 4 || !now.Before(entry.cleanupDeadline)
		common.SysError("Bangumi torrent "+entry.Hash[:12]+": owned cleanup failed; bounded retry or restart required")
		_ = m.saveJournal()
		return err
	}
	m.startupPending = false
	for _, remaining := range m.entries { if remaining.metadata == nil { m.startupPending = true; break } }
	return nil
}

func (m *bangumiTorrentManager) sweep(ctx context.Context, now time.Time) error {
	if err := m.lock(ctx); err != nil { return err }
	defer m.unlock()
	firstError := m.reconcile(ctx)
	for _, entry := range m.entries {
		if entry.CleanupPending || (len(entry.refs) == 0 && !entry.idleSince.IsZero() && now.Sub(entry.idleSince) >= bangumiTorrentIdleTTL) {
			if err := m.tryCleanup(ctx, entry, now, false); err != nil && firstError == nil { firstError = err }
		}
	}
	return firstError
}

func (m *bangumiTorrentManager) abandon(hash string) {
	ctx, cancel := context.WithTimeout(context.Background(), bangumiTorrentControlTimeout)
	defer cancel()
	if err := m.lock(ctx); err != nil { return }
	defer m.unlock()
	if entry := m.entries[hash]; entry != nil && len(entry.refs) == 0 { m.markFault(entry, m.now()); _ = m.tryCleanup(ctx, entry, m.now(), true) }
}

func (m *bangumiTorrentManager) streamTarget(id, index int) string {
	target := *m.backend
	target.Path, target.RawPath, target.RawQuery = fmt.Sprintf("/torrents/%d/stream/%d", id, index), "", ""
	return target.String()
}

func bangumiTorrentHash(hash string) bool {
	if len(hash) != 40 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil && hash == strings.ToLower(hash)
}

func bangumiNormalizeMagnet(raw string) (string, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "magnet" || parsed.User != nil || parsed.Host != "" || parsed.Path != "" || parsed.Fragment != "" || len(raw) > 8192 {
		return "", "", errors.New("Invalid magnet location")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query["xt"]) != 1 {
		return "", "", errors.New("Magnet must contain exactly one BTIH identity")
	}
	xt, ok := strings.CutPrefix(query.Get("xt"), "urn:btih:")
	if !ok {
		return "", "", errors.New("Only BTIH v1 magnets are supported")
	}
	var hash []byte
	if len(xt) == 40 {
		hash, err = hex.DecodeString(xt)
	} else if len(xt) == 32 {
		hash, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(xt))
	} else {
		err = errors.New("invalid hash size")
	}
	if err != nil || len(hash) != 20 {
		return "", "", errors.New("Invalid BTIH identity")
	}
	canonical := hex.EncodeToString(hash)
	// No tracker, webseed, exact-source or acceptable-source URL from a website
	// is forwarded to the trusted internal backend. Public DHT resolves the hash.
	return "magnet:?xt=urn:btih:" + canonical, canonical, nil
}

func bangumiValidTorrentFilename(name string) bool {
	if name == "" || len(name) > 1024 || !utf8.ValidString(name) || strings.ContainsAny(name, "\x00\r\n\\:") || strings.HasPrefix(name, "/") || path.Clean(name) != name {
		return false
	}
	for segment := range strings.SplitSeq(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func bangumiTorrentVideo(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".mp4", ".m4v", ".mkv", ".webm", ".avi", ".mov", ".ts", ".m2ts", ".flv", ".wmv":
		return true
	default:
		return false
	}
}

// The scanner preserves the exact original info dictionary bytes. Re-encoding
// info would change its BTIH and could attach a selection to a different swarm.
type bangumiBencodeValue struct {
	kind       byte
	data       []byte
	integer    int64
	list       []bangumiBencodeValue
	dict       map[string]bangumiBencodeValue
	start, end int
}
type bangumiBencodeScanner struct {
	body          []byte
	offset, nodes int
}

func (scanner *bangumiBencodeScanner) value(depth int) (bangumiBencodeValue, error) {
	if depth > 32 || scanner.nodes >= 16384 || scanner.offset >= len(scanner.body) {
		return bangumiBencodeValue{}, errors.New("Invalid or oversized torrent metadata structure")
	}
	scanner.nodes++
	value := bangumiBencodeValue{kind: scanner.body[scanner.offset], start: scanner.offset}
	switch value.kind {
	case 'i':
		scanner.offset++
		end := bytes.IndexByte(scanner.body[scanner.offset:], 'e')
		if end < 0 || end > 20 {
			return value, errors.New("Invalid torrent integer")
		}
		raw := string(scanner.body[scanner.offset : scanner.offset+end])
		if raw == "" || raw == "-0" || (len(raw) > 1 && raw[0] == '0') {
			return value, errors.New("Invalid torrent integer")
		}
		integer, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return value, errors.New("Invalid torrent integer")
		}
		value.integer = integer
		scanner.offset += end + 1
	case 'l', 'd':
		scanner.offset++
		if value.kind == 'd' {
			value.dict = make(map[string]bangumiBencodeValue)
		}
		for scanner.offset < len(scanner.body) && scanner.body[scanner.offset] != 'e' {
			if value.kind == 'l' {
				child, err := scanner.value(depth + 1)
				if err != nil {
					return value, err
				}
				value.list = append(value.list, child)
			} else {
				key, err := scanner.value(depth + 1)
				if err != nil || key.kind < '0' || key.kind > '9' || len(key.data) > 256 {
					return value, errors.New("Invalid torrent dictionary key")
				}
				name := string(key.data)
				if _, exists := value.dict[name]; exists {
					return value, errors.New("Duplicate torrent dictionary key")
				}
				child, err := scanner.value(depth + 1)
				if err != nil {
					return value, err
				}
				value.dict[name] = child
			}
		}
		if scanner.offset >= len(scanner.body) {
			return value, errors.New("Truncated torrent metadata")
		}
		scanner.offset++
	default:
		end := bytes.IndexByte(scanner.body[scanner.offset:], ':')
		if value.kind < '0' || value.kind > '9' || end < 1 || end > 7 {
			return value, errors.New("Invalid torrent string")
		}
		raw := string(scanner.body[scanner.offset : scanner.offset+end])
		if len(raw) > 1 && raw[0] == '0' {
			return value, errors.New("Invalid torrent string length")
		}
		length, err := strconv.Atoi(raw)
		scanner.offset += end + 1
		if err != nil || length < 0 || length > len(scanner.body)-scanner.offset {
			return value, errors.New("Invalid torrent string length")
		}
		value.data = scanner.body[scanner.offset : scanner.offset+length]
		scanner.offset += length
	}
	value.end = scanner.offset
	return value, nil
}

func bangumiSanitizeMetainfo(body []byte) ([]byte, string, []bangumiTorrentFile, error) {
	if len(body) == 0 || len(body) > bangumiTorrentBodyLimit {
		return nil, "", nil, errors.New("Invalid or oversized torrent metadata")
	}
	scanner := bangumiBencodeScanner{body: body}
	root, err := scanner.value(0)
	if err != nil || root.kind != 'd' || scanner.offset != len(body) {
		return nil, "", nil, errors.New("Invalid torrent metadata")
	}
	info, exists := root.dict["info"]
	if !exists || info.kind != 'd' {
		return nil, "", nil, errors.New("Torrent is missing its info dictionary")
	}
	name := string(info.dict["name"].data)
	if !bangumiValidTorrentFilename(name) || strings.Contains(name, "/") {
		return nil, "", nil, errors.New("Unsafe torrent root filename")
	}
	pieces := info.dict["pieces"]
	pieceLength := info.dict["piece length"].integer
	if len(pieces.data) == 0 || len(pieces.data)%20 != 0 || pieceLength <= 0 || pieceLength > 64<<20 {
		return nil, "", nil, errors.New("Unsupported torrent piece metadata")
	}
	files := []bangumiTorrentFile{}
	var total int64
	if listed, exists := info.dict["files"]; exists {
		if listed.kind != 'l' || len(listed.list) == 0 || len(listed.list) > bangumiTorrentFileLimit {
			return nil, "", nil, errors.New("Invalid torrent file list")
		}
		for _, file := range listed.list {
			if file.kind != 'd' || strings.Contains(string(file.dict["attr"].data), "l") {
				return nil, "", nil, errors.New("Torrent symlink files are not supported")
			}
			if _, linked := file.dict["symlink path"]; linked {
				return nil, "", nil, errors.New("Torrent symlink files are not supported")
			}
			components := file.dict["path"]
			if components.kind != 'l' || len(components.list) == 0 || len(components.list) > 32 {
				return nil, "", nil, errors.New("Invalid torrent path components")
			}
			parts := []string{name}
			for _, component := range components.list {
				part := string(component.data)
				if !bangumiValidTorrentFilename(part) || strings.Contains(part, "/") {
					return nil, "", nil, errors.New("Unsafe torrent filename")
				}
				parts = append(parts, part)
			}
			size := file.dict["length"].integer
			if size < 0 || size > (1<<62)-total {
				return nil, "", nil, errors.New("Invalid torrent file size")
			}
			total += size
			files = append(files, bangumiTorrentFile{Name: strings.Join(parts, "/"), Length: size})
		}
	} else {
		if _, linked := info.dict["symlink path"]; linked {
			return nil, "", nil, errors.New("Torrent symlink files are not supported")
		}
		length, exists := info.dict["length"]
		if !exists || length.kind != 'i' || length.integer <= 0 {
			return nil, "", nil, errors.New("Invalid torrent file size")
		}
		total = length.integer
		files = append(files, bangumiTorrentFile{Name: name, Length: total})
	}
	if total <= 0 || (total-1)/pieceLength+1 != int64(len(pieces.data)/20) {
		return nil, "", nil, errors.New("Torrent piece count does not match its files")
	}
	digest := sha1.Sum(body[info.start:info.end])
	sanitized := append([]byte("d4:info"), body[info.start:info.end]...)
	sanitized = append(sanitized, 'e')
	return sanitized, hex.EncodeToString(digest[:]), files, nil
}

func (m *bangumiTorrentManager) loadMetadata(ctx context.Context, uri string, headers http.Header) (*bangumiTorrentMetadata, error) {
	if uri == "" || len(uri) > 8192 {
		return nil, errors.New("Resolved torrent location is missing or invalid")
	}
	if strings.HasPrefix(uri, "magnet:") {
		canonical, _, err := bangumiNormalizeMagnet(uri)
		if err != nil {
			return nil, err
		}
		uri = canonical
	} else {
		parsed, err := url.Parse(uri)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || strings.ContainsAny(uri, "\r\n\x00\\") {
			return nil, errors.New("Invalid resolved torrent metadata location")
		}
	}
	digest := sha256.Sum256([]byte(uri))
	key := hex.EncodeToString(digest[:])
	m.mu.Lock()
	now := m.now()
	if cached := m.metadata[key]; cached != nil && cached.expires.After(now) {
		m.mu.Unlock()
		return cached, nil
	}
	if waiting := m.pending[key]; waiting != nil {
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, errors.New("Torrent metadata request canceled or expired")
		case <-waiting.done:
			return waiting.metadata, waiting.err
		}
	}
	if len(m.pending) >= 16 {
		m.mu.Unlock()
		return nil, errors.New("Torrent metadata capacity reached")
	}
	waiting := &bangumiTorrentMetadataWait{done: make(chan struct{})}
	m.pending[key] = waiting
	m.mu.Unlock()
	metadata, err := m.fetchMetadata(ctx, uri, headers)
	m.mu.Lock()
	delete(m.pending, key)
	waiting.metadata, waiting.err = metadata, err
	if err == nil {
		for id, cached := range m.metadata {
			if !cached.expires.After(m.now()) {
				delete(m.metadata, id)
			}
		}
		if len(m.metadata) >= 16 {
			oldestKey := ""
			var oldest time.Time
			for id, cached := range m.metadata {
				if oldestKey == "" || cached.expires.Before(oldest) {
					oldestKey, oldest = id, cached.expires
				}
			}
			delete(m.metadata, oldestKey)
		}
		m.metadata[key] = metadata
	}
	close(waiting.done)
	m.mu.Unlock()
	return metadata, err
}

func (m *bangumiTorrentManager) fetchMetadata(ctx context.Context, uri string, headers http.Header) (*bangumiTorrentMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	metadata := &bangumiTorrentMetadata{expires: m.now().Add(bangumiTorrentSelectionTTL)}
	var expected []bangumiTorrentFile
	if strings.HasPrefix(uri, "magnet:") {
		canonical, hash, err := bangumiNormalizeMagnet(uri)
		if err != nil {
			return nil, err
		}
		metadata.body, metadata.hash, metadata.isURL = []byte(canonical), hash, true
	} else {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
		if err != nil {
			return nil, errors.New("Cannot construct protected torrent metadata request")
		}
		request.Header = headers.Clone()
		if request.Header == nil {
			request.Header = make(http.Header)
		}
		request.Header.Set("Accept", "application/x-bittorrent, application/octet-stream")
		client := *m.publicClient
		client.Jar = nil
		client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
			if len(via) >= 5 || (next.URL.Scheme != "http" && next.URL.Scheme != "https") || next.URL.User != nil {
				return errors.New("Invalid torrent metadata redirect")
			}
			if m.publicClient.CheckRedirect != nil {
				if err := m.publicClient.CheckRedirect(next, via); err != nil {
					return err
				}
			}
			if !strings.EqualFold(next.URL.Host, request.URL.Host) {
				next.Header.Del("Cookie")
				next.Header.Del("Authorization")
			}
			return nil
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, errors.New("Protected torrent metadata request failed")
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, errors.New("Torrent metadata source rejected the request")
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, bangumiTorrentBodyLimit+1))
		if err != nil || len(body) > bangumiTorrentBodyLimit {
			return nil, errors.New("Invalid or oversized torrent metadata response")
		}
		metadata.body, metadata.hash, expected, err = bangumiSanitizeMetainfo(body)
		if err != nil {
			return nil, err
		}
	}
	query := url.Values{"list_only": {"true"}, "is_url": {strconv.FormatBool(metadata.isURL)}}
	var listed struct {
		Details bangumiTorrentDetail `json:"details"`
	}
	if err := m.backendRequest(ctx, http.MethodPost, "/torrents", query, metadata.body, &listed); err != nil {
		return nil, err
	}
	if !strings.EqualFold(listed.Details.InfoHash, metadata.hash) {
		return nil, errors.New("Torrent metadata identity does not match the resolved source")
	}
	if len(listed.Details.Files) == 0 || len(listed.Details.Files) > bangumiTorrentFileLimit {
		return nil, errors.New("Torrent backend returned an invalid file list")
	}
	if expected != nil && len(expected) != len(listed.Details.Files) {
		return nil, errors.New("Torrent backend file list does not match its metainfo")
	}
	seen := make(map[string]bool)
	for index, file := range listed.Details.Files {
		key := strings.ToLower(file.Name)
		if !bangumiValidTorrentFilename(file.Name) || file.Length < 0 || seen[key] { return nil, errors.New("Torrent contains an unsafe or aliased file path") }
		seen[key] = true
		if expected != nil {
			canonical := expected[index].Name
			_, relative, multi := strings.Cut(canonical, "/")
			if file.Length != expected[index].Length || (file.Name != canonical && (!multi || file.Name != relative)) { return nil, errors.New("Torrent backend file order does not match its metainfo") }
			metadata.diskNames = append(metadata.diskNames, canonical)
		}
		file.Name = strings.Clone(file.Name)
		listed.Details.Files[index] = file
	}
	metadata.files = listed.Details.Files
	return metadata, nil
}

func (m *bangumiTorrentManager) registerChoices(sourceID, episodeID string, metadata *bangumiTorrentMetadata, videos []int) ([]bangumiTorrentFileChoice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	existing := make(map[int]string)
	for id, binding := range m.bindings {
		if !binding.expires.After(now) {
			delete(m.bindings, id)
			continue
		}
		if binding.sourceID == sourceID && binding.episodeID == episodeID && binding.hash == metadata.hash {
			existing[binding.index] = id
		}
	}
	files := make([]bangumiTorrentFileChoice, 0, len(videos))
	for _, index := range videos {
		id := existing[index]
		if id == "" {
			if len(m.bindings) >= 4096 {
				return nil, errors.New("Torrent file selection capacity reached")
			}
			id = rand.Text()
			m.bindings[id] = bangumiTorrentFileBinding{sourceID: sourceID, episodeID: episodeID, hash: metadata.hash, index: index, expires: now.Add(bangumiTorrentSelectionTTL)}
		}
		file := metadata.files[index]
		files = append(files, bangumiTorrentFileChoice{ID: id, Name: file.Name, Size: file.Length})
	}
	return files, nil
}

func (m *bangumiTorrentManager) prepare(ctx context.Context, resolved service.BangumiSourceResolveResult, episodeID, fileID string) (bangumiTorrentPreparation, error) {
	result := bangumiTorrentPreparation{SourceID: resolved.SourceID, State: service.BangumiSourceFailed, Format: "torrent"}
	if resolved.SourceID == "" || len(resolved.SourceID) > 128 || episodeID == "" || len(episodeID) > 128 || resolved.Format != "torrent" || resolved.State != service.BangumiSourceReady || resolved.TorrentURI == "" || len(fileID) > 128 {
		return result, errors.New("Invalid resolved torrent source or episode")
	}
	var binding bangumiTorrentFileBinding
	if fileID != "" {
		m.mu.Lock()
		candidate, found := m.bindings[fileID]
		m.mu.Unlock()
		if !found || candidate.sourceID != resolved.SourceID || candidate.episodeID != episodeID || !candidate.expires.After(m.now()) {
			return result, errors.New("Unknown, expired or unrelated torrent file selection")
		}
		binding = candidate
	}
	if err := m.lock(ctx); err != nil {
		return result, err
	}
	err := m.reconcile(ctx)
	m.unlock()
	if err != nil {
		return result, err
	}
	metadata, err := m.loadMetadata(ctx, resolved.TorrentURI, resolved.Headers)
	if err != nil {
		return result, err
	}
	videos := []int{}
	for index, file := range metadata.files {
		if bangumiTorrentVideo(file.Name) && file.Length > 0 {
			videos = append(videos, index)
		}
	}
	if len(videos) == 0 {
		return result, errors.New("Torrent has no legal video file; no fallback file will be played")
	}
	index := -1
	if fileID != "" {
		if binding.hash != metadata.hash || !slices.Contains(videos, binding.index) {
			return result, errors.New("Selected file belongs to a different torrent or is not a video")
		}
		index = binding.index
	} else if len(videos) == 1 {
		index = videos[0]
	} else {
		result.Files, err = m.registerChoices(resolved.SourceID, episodeID, metadata, videos)
		if err != nil {
			return result, err
		}
		result.State = "selection_required"
		return result, nil
	}
	if err := m.lock(ctx); err != nil {
		return result, err
	}
	defer m.unlock()
	entry := m.entries[metadata.hash]
	if entry == nil {
		// Eviction is allowed only when no Range/HEAD currently references an
		// entry, and only after its last stream's full idle interval has elapsed.
		for _, candidate := range m.entries {
			if len(candidate.refs) == 0 && !candidate.idleSince.IsZero() && m.now().Sub(candidate.idleSince) >= bangumiTorrentIdleTTL {
				if err := m.deleteOwned(ctx, candidate); err != nil {
					return result, err
				}
			}
		}
		if len(m.entries) >= 2 || metadata.files[index].Length > m.maxBytes-m.bytes {
			return result, errors.New("Torrent activity or retained-byte budget reached")
		}
		torrents, err := m.listBackend(ctx)
		if err != nil {
			return result, err
		}
		for _, candidate := range torrents {
			if strings.EqualFold(candidate.InfoHash, metadata.hash) {
				return result, errors.New("Torrent is already managed outside this playback resource")
			}
		}
		folder := m.ownedFolder(metadata.hash)
		if _, err := os.Lstat(folder); err == nil || !os.IsNotExist(err) { return result, errors.New("Unproven torrent output directory will not be reused or overwritten") }
		entry = &bangumiTorrentOwned{Hash: metadata.hash, Retained: map[int]int64{index: metadata.files[index].Length}, metadata: metadata, wanted: index, included: []int{index}, refs: make(map[int]int), idleSince: m.now()}
		m.entries[metadata.hash] = entry
		m.bytes += metadata.files[index].Length
		if err := m.saveJournal(); err != nil {
			delete(m.entries, metadata.hash)
			m.bytes -= metadata.files[index].Length
			return result, err
		}
		if err := os.Mkdir(folder, 0700); err != nil {
			// Failed exclusive creation proves neither directory nor backend
			// ownership. Roll back only this admission's budget and intent.
			delete(m.entries, metadata.hash)
			m.bytes -= metadata.files[index].Length
			_ = m.saveJournal()
			return result, errors.New("Owned torrent output directory is unavailable or already exists")
		}
		entry.DirectoryOwned = true
		if err := bangumiTorrentWriteJSON(filepath.Join(folder, ".rex-owner.json"), bangumiTorrentDirectoryOwner{Owner: m.owner, Hash: entry.Hash}); err != nil {
			m.rollbackAdmission(entry)
			return result, err
		}
		if err := m.saveJournal(); err != nil { m.rollbackAdmission(entry); return result, err }
		query := url.Values{"only_files": {strconv.Itoa(index)}, "output_folder": {folder}, "is_url": {strconv.FormatBool(metadata.isURL)}, "overwrite": {"false"}}
		var added struct {
			ID      *int                 `json:"id"`
			Details bangumiTorrentDetail `json:"details"`
		}
		if err := m.backendRequest(ctx, http.MethodPost, "/torrents", query, metadata.body, &added); err != nil {
			m.rollbackAdmission(entry)
			return result, err
		}
		if !m.ownsDetail(added.Details, metadata.hash) || (added.ID != nil && (added.Details.ID == nil || *added.ID != *added.Details.ID)) || !bangumiTorrentIncluded(added.Details.Files, metadata.files, []int{index}) {
			m.rollbackAdmission(entry)
			return result, errors.New("Torrent backend did not admit the exact requested file and owned output")
		}
		entry.ID = added.Details.ID
		if err := m.saveJournal(); err != nil {
			m.rollbackAdmission(entry)
			return result, err
		}
	} else {
		if entry.ID == nil || entry.metadata == nil {
			return result, errors.New("Torrent admission or cleanup is pending")
		}
		if len(entry.metadata.files) != len(metadata.files) {
			return result, errors.New("Torrent metadata changed unexpectedly")
		}
		for i, file := range metadata.files {
			if file.Name != entry.metadata.files[i].Name || file.Length != entry.metadata.files[i].Length {
				return result, errors.New("Torrent metadata changed unexpectedly")
			}
		}
		if err := m.selectFile(ctx, entry, index); err != nil {
			return result, err
		}
	}
	result.State, result.target, result.hash, result.fileIndex = service.BangumiSourceReady, m.streamTarget(*entry.ID, index), entry.Hash, index
	backendID := *entry.ID
	result.onStart = func() (func(), error) { return m.startStream(metadata.hash, index, backendID) }
	return result, nil
}

func bangumiTorrentIncluded(actual, expected []bangumiTorrentFile, only []int) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i, file := range actual {
		if file.Name != expected[i].Name || file.Length != expected[i].Length || file.Included != slices.Contains(only, i) {
			return false
		}
	}
	return true
}

func (m *bangumiTorrentManager) rollbackAdmission(entry *bangumiTorrentOwned) {
	ctx, cancel := context.WithTimeout(context.Background(), bangumiTorrentControlTimeout)
	defer cancel()
	// A failed HTTP add can still have reached the backend. Its persisted intent
	// and output path identify it for delete; budget is held if cleanup fails.
	m.markFault(entry, m.now())
	_ = m.tryCleanup(ctx, entry, m.now(), true)
}

func (m *bangumiTorrentManager) desiredFiles(entry *bangumiTorrentOwned, wanted int) []int {
	files := []int{wanted}
	for index, references := range entry.refs {
		if references > 0 && index != wanted {
			files = append(files, index)
		}
	}
	slices.Sort(files)
	return files
}

func (m *bangumiTorrentManager) updateFiles(ctx context.Context, entry *bangumiTorrentOwned, wanted int) error {
	files := m.desiredFiles(entry, wanted)
	// rqbit may restart and reuse numeric IDs. Validate ownership before either
	// reusing a grant or changing a backend selection, including the no-op path.
	detail, err := m.detail(ctx, *entry.ID)
	if err != nil || !m.ownsDetail(detail, entry.Hash) || len(detail.Files) != len(entry.metadata.files) { return errors.New("Torrent backend resource identity changed or expired") }
	for i, file := range detail.Files { if file.Name != entry.metadata.files[i].Name || file.Length != entry.metadata.files[i].Length { return errors.New("Torrent backend file layout changed") } }
	if bangumiTorrentIncluded(detail.Files, entry.metadata.files, files) { entry.included = files; return nil }
	body, err := common.Marshal(map[string]any{"only_files": files})
	if err != nil { return errors.New("Cannot encode exact torrent file selection") }
	if err := m.backendRequest(ctx, http.MethodPost, fmt.Sprintf("/torrents/%d/update_only_files", *entry.ID), nil, body, nil); err != nil { return err }
	detail, err = m.detail(ctx, *entry.ID)
	if err != nil || !m.ownsDetail(detail, entry.Hash) || !bangumiTorrentIncluded(detail.Files, entry.metadata.files, files) { return errors.New("Torrent backend did not retain the exact active file selection") }
	entry.included = files
	return nil
}

func (m *bangumiTorrentManager) selectFile(ctx context.Context, entry *bangumiTorrentOwned, index int) error {
	if entry.CleanupPending { return errors.New("Faulted torrent resource is draining; new Range/HEAD requests are denied") }
	if index < 0 || index >= len(entry.metadata.files) { return errors.New("Invalid torrent file index") }
	previous, newReservation := entry.wanted, false
	if _, retained := entry.Retained[index]; !retained {
		size := entry.metadata.files[index].Length
		if size <= 0 || size > m.maxBytes-m.bytes { return errors.New("Torrent retained-byte budget reached before file selection") }
		entry.Retained[index] = size
		m.bytes += size
		newReservation = true
		if err := m.saveJournal(); err != nil { delete(entry.Retained, index); m.bytes -= size; return err }
	}
	if err := m.updateFiles(ctx, entry, index); err != nil {
		m.markFault(entry, m.now())
		if len(entry.refs) == 0 { m.rollbackAdmission(entry) } else {
			// Restore the exact old union without canceling an active reader.
			// Only a confirmed, never-created file can release its reservation.
			entry.included = nil
			rollbackCtx, cancel := context.WithTimeout(context.Background(), bangumiTorrentControlTimeout)
			restored := m.updateFiles(rollbackCtx, entry, previous) == nil
			cancel()
			if restored && newReservation && m.unwrittenFile(entry, index) {
				m.bytes -= entry.Retained[index]
				delete(entry.Retained, index)
				_ = m.saveJournal()
			}
		}
		return err
	}
	entry.wanted = index
	if len(entry.refs) == 0 { entry.idleSince = m.now() }
	return nil
}

func (m *bangumiTorrentManager) unwrittenFile(entry *bangumiTorrentOwned, index int) bool {
	if !entry.DirectoryOwned || index < 0 || index >= len(entry.metadata.diskNames) { return false }
	for _, name := range []string{entry.metadata.diskNames[index], entry.metadata.files[index].Name} {
		if !bangumiValidTorrentFilename(name) { return false }
		folder := m.ownedFolder(entry.Hash)
		parts := strings.Split(name, "/")
		for _, segment := range parts[:len(parts)-1] {
			folder = filepath.Join(folder, segment)
			if info, err := os.Lstat(folder); err == nil { if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 { return false } } else if !os.IsNotExist(err) { return false }
		}
		if _, err := os.Lstat(filepath.Join(m.ownedFolder(entry.Hash), filepath.FromSlash(name))); !os.IsNotExist(err) { return false }
	}
	return true
}

func (m *bangumiTorrentManager) startStream(hash string, index, backendID int) (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), bangumiTorrentControlTimeout)
	defer cancel()
	if err := m.lock(ctx); err != nil {
		return nil, err
	}
	defer m.unlock()
	entry := m.entries[hash]
	if entry == nil || entry.ID == nil || entry.metadata == nil || index < 0 || index >= len(entry.metadata.files) {
		return nil, errors.New("Torrent playback resource has expired")
	}
	// The grant was issued against one backend id. If that id has since been
	// released and reused by a different torrent, this callback would otherwise
	// stream bytes from an unrelated release, so it fails closed instead.
	if *entry.ID != backendID {
		return nil, errors.New("Torrent playback resource has been replaced")
	}
	if entry.refs[index] >= 32 {
		return nil, errors.New("Torrent active stream capacity reached")
	}
	// A previous grant may be used again after switching files. Include it with
	// every still-active reader before forwarding its Range or HEAD request.
	if err := m.selectFile(ctx, entry, index); err != nil {
		return nil, err
	}
	entry.refs[index]++
	entry.idleSince = time.Time{}
	var once sync.Once
	return func() { once.Do(func() { m.finishStream(hash, index) }) }, nil
}

func (m *bangumiTorrentManager) finishStream(hash string, index int) {
	// Completion must never lose a reference because another bounded control
	// operation is in flight. Waiting here is conservative: cleanup cannot run
	// before the completed reference has been accounted for.
	m.control <- struct{}{}
	defer m.unlock()
	ctx, cancel := context.WithTimeout(context.Background(), bangumiTorrentControlTimeout)
	defer cancel()
	entry := m.entries[hash]
	if entry == nil || entry.refs[index] == 0 {
		return
	}
	entry.refs[index]--
	if entry.refs[index] == 0 {
		delete(entry.refs, index)
	}
	if len(entry.refs) == 0 {
		entry.idleSince = m.now()
		if entry.CleanupPending { _ = m.tryCleanup(ctx, entry, m.now(), true); return }
	}
	if err := m.updateFiles(ctx, entry, entry.wanted); err != nil {
		entry.included = nil
		m.markFault(entry, m.now())
	}
}
