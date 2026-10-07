package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBangumiTestManager(t *testing.T, client *http.Client, base string, config map[string]any) *BangumiSourceManager {
	t.Helper()
	defaults := map[string]any{
		"searchUrl": base + "/search?q={keyword}", "requestInterval": 0,
		"searchUseOnlyFirstWord": false, "searchRemoveSpecial": false,
		"subjectFormatId": "a", "selectorSubjectFormatA": map[string]any{"selectLists": ".result a"},
		"channelFormatId": "no-channel", "selectorChannelFormatNoChannel": map[string]any{"selectEpisodes": ".episodes a"},
		"matchVideo": map[string]any{"matchNestedUrl": "$^", "matchVideoUrl": `(?<v>https?://[^\s]+\.(?:m3u8|mp4|mkv)(?:\?[^\s]*)?)`},
	}
	for key, value := range config {
		defaults[key] = value
	}
	factory, kind := "web-selector", "web"
	if value, ok := defaults["factory"].(string); ok {
		factory = value
		delete(defaults, "factory")
		kind = "bt"
	}
	catalog := map[string]any{"version": 1, "sources": []any{map[string]any{"mediaSourceId": "fixture", "name": "Fixture", "factoryId": factory, "kind": kind, "enabled": true, "searchConfig": defaults}}}
	encoded, err := common.Marshal(catalog)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "catalog.json")
	require.NoError(t, os.WriteFile(path, encoded, 0600))
	manager, err := NewBangumiSourceManager(client, path)
	require.NoError(t, err)
	return manager
}

func browseBangumiFixture(t *testing.T, manager *BangumiSourceManager) BangumiSourceBrowseResult {
	t.Helper()
	searched, err := manager.Search(context.Background(), "fixture", "动漫")
	require.NoError(t, err)
	require.Len(t, searched.Subjects, 1)
	browsed, err := manager.Browse(context.Background(), "fixture", searched.Subjects[0].ID)
	require.NoError(t, err)
	return browsed
}

func TestBangumiSourcesCatalogMetadata(t *testing.T) {
	manager, err := NewBangumiSourceManager(&http.Client{}, "")
	require.NoError(t, err)
	sources := manager.Sources()
	require.Len(t, sources, 22)
	expected := []string{"蜜柑计划 (CN)", "動漫花園", "nyaa.land", "AnimeGarden", "稀饭动漫", "girigiri愛動漫", "嘀嗒影视", "叽哔动漫", "E-ACG", "森之屋动漫", "风车影视", "去看吧", "海星动漫", "樱花动漫", "嘀哩嘀哩", "新优酷", "番茄动漫", "hanime1_1080p", "hanime1_720p", "wedm", "影视森林", "热播之家"}
	web, bt := 0, 0
	ids := make(map[string]bool)
	for i, source := range sources {
		assert.Equal(t, expected[i], source.Name)
		assert.NotEmpty(t, source.ID)
		assert.NotEmpty(t, source.Icon)
		assert.True(t, source.Enabled)
		assert.False(t, ids[source.ID], "source IDs must be unique")
		ids[source.ID] = true
		if source.Kind == "web" {
			web++
		} else if source.Kind == "bt" {
			bt++
		}
	}
	assert.Equal(t, 18, web)
	assert.Equal(t, 4, bt)
	encoded, err := common.Marshal(sources)
	require.NoError(t, err)
	var metadata []map[string]any
	require.NoError(t, common.Unmarshal(encoded, &metadata))
	for _, source := range metadata {
		assert.Len(t, source, 7)
		assert.NotContains(t, source, "searchConfig")
		assert.NotContains(t, source, "cookies")
		// The picker needs a site to send a viewer to when a source answers with
		// a challenge, so every source exposes one.
		website, ok := source["websiteUrl"].(string)
		assert.True(t, ok, "source %v has no website", source["id"])
		assert.True(t, strings.HasPrefix(website, "http://") || strings.HasPrefix(website, "https://"))
	}
	_, err = NewBangumiSourceManager(nil, "")
	require.Error(t, err)
	result, err := manager.Search(context.Background(), "mikan-mikanime-tv", "动漫")
	require.Error(t, err)
	assert.Equal(t, BangumiSourceInvalidConfig, result.State)
	assert.Equal(t, "builtin_adapter_required", result.Message)
	unknown, err := manager.Search(context.Background(), "unknown", "动漫")
	require.Error(t, err)
	assert.Equal(t, BangumiSourceInvalidConfig, unknown.State)
}

func TestBangumiSourceSearchFormatsAndOpaqueCache(t *testing.T) {
	cases := []struct {
		name, body string
		config     map[string]any
		names      []string
	}{
		{"a", `<html><div class="result"><a href="/subject/1" title="标题优先">正文</a><a href="/subject/2">第二条</a></div></html>`, nil, []string{"标题优先", "第二条"}},
		{"bounded Unicode titles", fmt.Sprintf(`<div class="result"><a href="/subject/1" title="%s">正文</a><a href="/subject/2">%s</a></div>`, strings.Repeat(" ", 512)+"SP 特别篇"+strings.Repeat(" ", 512), strings.Repeat("长", 513)), nil, []string{"SP 特别篇", strings.Repeat("长", 512)}},
		{"indexed", `<html><div class="names"><h2>第一条</h2><h2>第二条</h2></div><div class="links"><a href="/subject/1"></a><a href="/subject/2"></a></div></html>`, map[string]any{"subjectFormatId": "indexed", "selectorSubjectFormatIndexed": map[string]any{"selectNames": ".names h2", "selectLinks": ".links a"}}, []string{"第一条", "第二条"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, "中文 + &/", r.URL.Query().Get("q"))
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			manager := newBangumiTestManager(t, server.Client(), server.URL, tc.config)
			result, err := manager.Search(context.Background(), "fixture", "中文 + &/")
			require.NoError(t, err)
			assert.Equal(t, BangumiSourceReady, result.State)
			require.Len(t, result.Subjects, 2)
			for i, subject := range result.Subjects {
				assert.Equal(t, tc.names[i], subject.Name)
				assert.NotContains(t, subject.ID, server.URL)
				assert.NotContains(t, subject.ID, "/subject/")
			}
			firstID := result.Subjects[0].ID
			result.Subjects[0].Name = "client mutation"
			cached, err := manager.Search(context.Background(), "fixture", "中文 + &/")
			require.NoError(t, err)
			assert.Equal(t, tc.names[0], cached.Subjects[0].Name)
			assert.Equal(t, firstID, cached.Subjects[0].ID)
			assert.EqualValues(t, 1, calls.Load())
			publicJSON, err := common.Marshal(cached)
			require.NoError(t, err)
			assert.NotContains(t, string(publicJSON), server.URL)
			assert.NotContains(t, string(publicJSON), "quality=")
			other := newBangumiTestManager(t, server.Client(), server.URL, map[string]any{"defaultResolution": "720P"})
			if tc.name == "a" {
				alternate, err := other.Search(context.Background(), "fixture", "中文 + &/")
				require.NoError(t, err)
				assert.NotEqual(t, firstID, alternate.Subjects[0].ID)
			}
			_, err = manager.Browse(context.Background(), "unknown", firstID)
			require.Error(t, err)
			_, err = manager.Resolve(context.Background(), "fixture", firstID)
			require.Error(t, err, "subject IDs cannot be used as episode IDs")
		})
	}
}

func TestBangumiSourceBrowseFormats(t *testing.T) {
	cases := []struct {
		name, body    string
		config        map[string]any
		labels, names []string
		numbers       [][]string
	}{
		{"grouped duplicates", `<html><div class="channels"><span>线路1</span><span>线路2</span></div><ul class="list"><li><a href="/play/1"><span>第01集</span></a></li><li><a href="/play/sp"><span>SP 2</span></a></li></ul><ul class="list"><li><a href="/play/2"><span>第十二点五集</span></a></li><li><a href="/play/3"><span>第12.5话</span></a></li></ul></html>`, map[string]any{"channelFormatId": "index-grouped", "selectorChannelFormatFlattened": map[string]any{"selectChannelNames": ".channels span", "matchChannelName": `^(?<ch>线路)\d+$`, "selectEpisodeLists": ".list", "selectEpisodesFromList": "a span", "selectEpisodeLinksFromList": "a", "matchEpisodeSortFromName": `第\s*(?<ep>.+?)\s*[话集]|(?<ep>SP\s\d+)`}}, []string{"线路1", "线路2"}, []string{"线路", "线路"}, [][]string{{"01", "SP 2"}, {"十二点五", "12.5"}}},
		{"grouped full labels", `<html><span class="channel">重复</span><span class="channel">重复</span><ul class="list"><a href="/1">正片</a></ul><ul class="list"><a href="/2">SP 特别篇</a></ul></html>`, map[string]any{"channelFormatId": "index-grouped", "selectorChannelFormatFlattened": map[string]any{"selectChannelNames": ".channel", "matchChannelName": "", "selectEpisodeLists": ".list", "selectEpisodesFromList": "a", "matchEpisodeSortFromName": ""}}, []string{"重复", "重复"}, []string{"重复", "重复"}, [][]string{{"正片"}, {"SP 特别篇"}}},
		{"no channel defaults", `<html><div class="episodes"><a href="/1">第零集</a><a href="/2">SP 特别篇</a></div></html>`, nil, []string{""}, []string{""}, [][]string{{"零", "SP 特别篇"}}},
		{"regex without named group", `<html><div class="episodes"><a href="/1">第01集</a></div></html>`, map[string]any{"selectorChannelFormatNoChannel": map[string]any{"selectEpisodes": ".episodes a", "matchEpisodeSortFromName": `\d+`}}, []string{""}, []string{""}, [][]string{{"第01集"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/search" {
					fmt.Fprint(w, `<div class="result"><a href="/subject">条目</a></div>`)
					return
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			manager := newBangumiTestManager(t, server.Client(), server.URL, tc.config)
			result := browseBangumiFixture(t, manager)
			assert.Equal(t, BangumiSourceReady, result.State)
			require.Len(t, result.Channels, len(tc.names))
			for i, channel := range result.Channels {
				assert.Equal(t, tc.names[i], channel.Name)
				assert.Equal(t, tc.labels[i], channel.Label)
				actual := []string{}
				for _, episode := range channel.Episodes {
					actual = append(actual, episode.EpisodeNumber)
				}
				assert.Equal(t, tc.numbers[i], actual)
			}
			if len(result.Channels) == 2 {
				assert.NotEqual(t, result.Channels[0].ID, result.Channels[1].ID)
				assert.NotEqual(t, result.Channels[0].Episodes[0].ID, result.Channels[1].Episodes[0].ID)
			}
			publicJSON, err := common.Marshal(result)
			require.NoError(t, err)
			assert.NotContains(t, string(publicJSON), server.URL)
		})
	}
}

func TestBangumiSourceFailureAndCaptchaStates(t *testing.T) {
	cases := []struct {
		name                        string
		status                      int
		body, retry, state, captcha string
		wantErr                     bool
	}{
		{"empty", 200, `<html><body>No results</body></html>`, "", BangumiSourceEmpty, "", false},
		{"word captcha is not a challenge", 200, `<html><body>captcha documentation</body></html>`, "", BangumiSourceEmpty, "", false},
		{"normal parser takes precedence", 200, `<div class="result"><a href="/subject">captcha anime</a></div><div class="cf-turnstile"></div>`, "", BangumiSourceReady, "", false},
		{"turnstile after empty parse", 200, `<html><div class="cf-turnstile"></div></html>`, "", BangumiSourceCaptchaRequired, "cloudflare_turnstile", true},
		{"cf blocked", 403, `<html><script>window._cf_chl_opt={}</script></html>`, "", BangumiSourceCaptchaRequired, "cloudflare", true},
		{"image structure", 200, `<input name="verify"><img class="ds-verify-img"><button class="verify-submit">提交验证</button>`, "", BangumiSourceCaptchaRequired, "image", true},
		{"403 alone is not captcha", 403, `<html>Denied</html>`, "", BangumiSourceFailed, "", true},
		{"429 independent of captcha", 429, `<div class="cf-turnstile"></div>`, "120", BangumiSourceRateLimited, "", true},
		{"bad markup", 200, `upstream crashed with quality=SECRET`, "", BangumiSourceFailed, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", tc.retry)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			manager := newBangumiTestManager(t, server.Client(), server.URL, nil)
			before := time.Now()
			result, err := manager.Search(context.Background(), "fixture", "动漫")
			if tc.wantErr {
				require.Error(t, err)
				var typed *BangumiSourceError
				require.ErrorAs(t, err, &typed)
				assert.Equal(t, result.State, typed.State)
				assert.NotContains(t, err.Error(), server.URL)
				assert.NotContains(t, err.Error(), "SECRET")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.state, result.State)
			assert.Equal(t, tc.captcha, result.CaptchaKind)
			if tc.retry != "" {
				require.NotNil(t, result.RetryAt)
				assert.WithinDuration(t, before.Add(120*time.Second), *result.RetryAt, time.Second)
			}
		})
	}
	t.Run("network failure is not empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		manager := newBangumiTestManager(t, server.Client(), server.URL, nil)
		server.Close()
		result, err := manager.Search(context.Background(), "fixture", "动漫")
		require.Error(t, err)
		assert.Equal(t, BangumiSourceFailed, result.State)
		assert.Empty(t, result.Subjects)
	})
	t.Run("Retry-After date", func(t *testing.T) {
		retry := time.Now().Add(5 * time.Minute).UTC().Truncate(time.Second)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", retry.Format(http.TimeFormat))
			w.WriteHeader(429)
		}))
		defer server.Close()
		result, err := newBangumiTestManager(t, server.Client(), server.URL, nil).Search(context.Background(), "fixture", "动漫")
		require.Error(t, err)
		require.NotNil(t, result.RetryAt)
		assert.True(t, retry.Equal(*result.RetryAt))
	})
}

func TestBangumiSourceInvalidRulesAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config map[string]any
	}{
		{"malformed regex", map[string]any{"matchVideo": map[string]any{"matchVideoUrl": "(?<v>"}}},
		{"malformed CSS", map[string]any{"selectorSubjectFormatA": map[string]any{"selectLists": "div["}}},
		{"unknown format", map[string]any{"subjectFormatId": "not-supported"}},
		{"unknown factory", map[string]any{"factory": "not-supported"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			defer server.Close()
			manager := newBangumiTestManager(t, server.Client(), server.URL, tc.config)
			result, err := manager.Search(context.Background(), "fixture", "动漫")
			require.Error(t, err)
			assert.Equal(t, BangumiSourceInvalidConfig, result.State)
			assert.EqualValues(t, 0, calls.Load())
		})
	}
	t.Run("regex timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/search" {
				fmt.Fprint(w, `<div class="result"><a href="/subject">条目</a></div>`)
				return
			}
			fmt.Fprintf(w, `<div class="episodes"><a href="/play">%s!</a></div>`, strings.Repeat("a", 2048))
		}))
		defer server.Close()
		manager := newBangumiTestManager(t, server.Client(), server.URL, map[string]any{"selectorChannelFormatNoChannel": map[string]any{"selectEpisodes": ".episodes a", "matchEpisodeSortFromName": `(a+)+$`}})
		searched, err := manager.Search(context.Background(), "fixture", "动漫")
		require.NoError(t, err)
		result, err := manager.Browse(context.Background(), "fixture", searched.Subjects[0].ID)
		require.Error(t, err)
		assert.Equal(t, BangumiSourceInvalidConfig, result.State)
		assert.Contains(t, result.Message, "timed out")
	})
	t.Run("canceled before request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("canceled request reached upstream") }))
		defer server.Close()
		manager := newBangumiTestManager(t, server.Client(), server.URL, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := manager.Search(ctx, "fixture", "动漫")
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, BangumiSourceFailed, result.State)
	})
	t.Run("canceled in flight", func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
		defer server.Close()
		manager := newBangumiTestManager(t, server.Client(), server.URL, nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { <-started; cancel() }()
		_, err := manager.Search(ctx, "fixture", "动漫")
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestBangumiSourceStaticResolution(t *testing.T) {
	cases := []struct {
		name          string
		pages         map[string]string
		format, state string
		config        map[string]any
	}{
		{"video source", map[string]string{"/play": `<video><source src="/media.m3u8?token=literal" type="application/x-mpegURL"></video>`}, "hls", BangumiSourceReady, nil},
		{"placeholder iframes do not poison video", map[string]string{"/play": `<video src="/media.mp4"></video><iframe src="about:blank"></iframe><iframe src="javascript:void(0)"></iframe><iframe src="file:///untrusted"></iframe>`}, "mp4", BangumiSourceReady, nil},
		{"iframe nested", map[string]string{"/play": `<iframe src="/nested"></iframe>`, "/nested": `<video src="/media.mp4"></video>`}, "mp4", BangumiSourceReady, nil},
		{"literal percent", map[string]string{"/play": `<script>var player_aaaa={"encrypt":1,"url":"{PERCENT}"};</script>`}, "mp4", BangumiSourceReady, nil},
		{"literal base64", map[string]string{"/play": `<script>var player_aaaa={"encrypt":2,"url":"{BASE64}"};</script>`}, "mp4", BangumiSourceReady, nil},
		{"single quoted literal", map[string]string{"/play": `<script>var player_aaaa={'url':'/media.mp4','encrypt':0};</script>`}, "mp4", BangumiSourceReady, nil},
		{"DOM scan", map[string]string{"/play": `<div data-video="/media.mkv"></div>`}, "matroska", BangumiSourceReady, map[string]any{"matchVideo": map[string]any{"scanDomMediaUrls": true, "matchNestedUrl": "$^"}}},
		{"inline scan", map[string]string{"/play": `<script>const urls=['{BASE}/media.mp4'];</script>`}, "mp4", BangumiSourceReady, map[string]any{"matchVideo": map[string]any{"scanInlineScriptUrls": true, "matchNestedUrl": "$^"}}},
		{"JS only", map[string]string{"/play": `<script>fetch('/signed').then(r=>r.json()).then(data=>video.src=data.url)</script><video></video>`}, "unknown", BangumiSourceBrowserRequired, nil},
		{"concatenation is not a literal", map[string]string{"/play": `<script>var player_aaaa={url:'{BASE}/media.mp4'+getSignature()};</script>`}, "unknown", BangumiSourceBrowserRequired, nil},
		{"lookahead nested rule", map[string]string{"/play": `<iframe src="/wrapper?url={BASE}/media.mp4"></iframe>`, "/wrapper": `<video src="/media.mp4"></video>`}, "mp4", BangumiSourceReady, map[string]any{"matchVideo": map[string]any{"matchNestedUrl": `^(?=.*wrapper)(?=.*url=).+`, "matchVideoUrl": `url=(?<v>https?://.+\.mp4)|^https?://.+\.mp4`}}},
		// A page often carries a placeholder host -- maccms prints
		// `"remoteurl":"http://img.test.com/"` in its own config -- alongside the
		// real player URL. The placeholder is extracted as a candidate too, and
		// before this case was fixed its unreachable host aborted the whole
		// resolution instead of being skipped for the next candidate.
		{"unreachable placeholder does not abort resolution", map[string]string{"/play": `<script>var config={"remoteurl":"http://127.0.0.1:1/dead.mp4"};</script><script>var player_bbbb={"url":"/media.mp4"};</script>`}, "mp4", BangumiSourceReady, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var base string
			var mediaProbes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/search":
					fmt.Fprint(w, `<div class="result"><a href="/subject">条目</a></div>`)
				case "/subject":
					fmt.Fprint(w, `<div class="episodes"><a href="/play">第01集</a></div>`)
				case "/media.mp4", "/media.m3u8", "/media.mkv":
					mediaProbes.Add(1)
					assert.Equal(t, http.MethodHead, r.Method)
					assert.Equal(t, "quality=1080", r.Header.Get("Cookie"))
					if strings.HasSuffix(r.URL.Path, ".m3u8") {
						w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
					} else if strings.HasSuffix(r.URL.Path, ".mkv") {
						w.Header().Set("Content-Type", "video/x-matroska")
					} else {
						w.Header().Set("Content-Type", "video/mp4")
					}
				default:
					w.Header().Set("Content-Type", "text/html")
					body := tc.pages[r.URL.Path]
					body = strings.NewReplacer("{BASE}", base, "{PERCENT}", url.PathEscape(base+"/media.mp4"), "{BASE64}", base64.StdEncoding.EncodeToString([]byte(base+"/media.mp4"))).Replace(body)
					fmt.Fprint(w, body)
				}
			}))
			defer server.Close()
			base = server.URL
			manager := newBangumiTestManager(t, server.Client(), base, tc.config)
			browse := browseBangumiFixture(t, manager)
			result, err := manager.Resolve(context.Background(), "fixture", browse.Channels[0].Episodes[0].ID)
			if tc.state == BangumiSourceReady {
				require.NoError(t, err)
				assert.NotEmpty(t, result.VideoURL)
				assert.EqualValues(t, 1, mediaProbes.Load())
				assert.NotEmpty(t, result.Headers.Get("Referer"))
				result.Headers.Set("Cookie", "client mutation")
				cached, err := manager.Resolve(context.Background(), "fixture", browse.Channels[0].Episodes[0].ID)
				require.NoError(t, err)
				assert.Equal(t, "quality=1080", cached.Headers.Get("Cookie"))
			} else {
				require.Error(t, err)
				assert.Empty(t, result.VideoURL)
				assert.EqualValues(t, 0, mediaProbes.Load())
			}
			assert.Equal(t, tc.state, result.State)
			assert.Equal(t, tc.format, result.Format)
			publicJSON, err := common.Marshal(result)
			require.NoError(t, err)
			assert.NotContains(t, string(publicJSON), base)
			assert.NotContains(t, string(publicJSON), "Cookie")
		})
	}
}

func TestBangumiSourceResolutionAndFetchLimits(t *testing.T) {
	for _, name := range []string{"depth", "requests", "unsafe URL", "userinfo", "oversized body", "captcha"} {
		t.Run(name, func(t *testing.T) {
			var parserCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/search" {
					fmt.Fprint(w, `<div class="result"><a href="/subject">条目</a></div>`)
					return
				}
				if r.URL.Path == "/subject" {
					fmt.Fprint(w, `<div class="episodes"><a href="/play">第1集</a></div>`)
					return
				}
				parserCalls.Add(1)
				switch name {
				case "depth":
					level := r.URL.Query().Get("level")
					next := "1"
					if level != "" {
						var n int
						fmt.Sscanf(level, "%d", &n)
						next = fmt.Sprint(n + 1)
					}
					fmt.Fprintf(w, `<iframe src="/play?level=%s"></iframe>`, next)
				case "requests":
					if r.URL.Path == "/play" {
						for i := range 12 {
							fmt.Fprintf(w, `<iframe src="/nested/%d"></iframe>`, i)
						}
					} else {
						fmt.Fprint(w, `<html>No static video</html>`)
					}
				case "unsafe URL":
					fmt.Fprint(w, `<video src="file:///etc/passwd"></video>`)
				case "userinfo":
					fmt.Fprint(w, `<video src="https://user:pass@cdn.invalid/media.mp4"></video>`)
				case "oversized body":
					fmt.Fprint(w, strings.Repeat("x", (2<<20)+1))
				case "captcha":
					fmt.Fprint(w, `<html><div class="cf-turnstile"></div></html>`)
				}
			}))
			defer server.Close()
			manager := newBangumiTestManager(t, server.Client(), server.URL, nil)
			browse := browseBangumiFixture(t, manager)
			result, err := manager.Resolve(context.Background(), "fixture", browse.Channels[0].Episodes[0].ID)
			require.Error(t, err)
			assert.Empty(t, result.VideoURL)
			if name == "depth" || name == "requests" {
				assert.Equal(t, BangumiSourceBrowserRequired, result.State)
				assert.Contains(t, result.Message, "limit")
				assert.LessOrEqual(t, parserCalls.Load(), int32(6))
			} else if name == "captcha" {
				assert.Equal(t, BangumiSourceCaptchaRequired, result.State)
				assert.Equal(t, "cloudflare_turnstile", result.CaptchaKind)
			} else {
				assert.Equal(t, BangumiSourceFailed, result.State)
			}
		})
	}
}

func TestBangumiSourceBTReleaseHandoff(t *testing.T) {
	const magnet = "magnet:?xt=urn:btih:0123456789012345678901234567890123456789"
	cases := []struct {
		name, factory, body             string
		expectedTorrent, expectedMagnet string
	}{
		{"rss torrent", "rss", `<rss version="2.0"><channel><item><title>动画 SP 2</title><link>/release/1</link><enclosure url="/release/1.torrent" type="application/x-bittorrent"/></item></channel></rss>`, "/release/1.torrent", ""},
		{"rss namespaced magnet", "rss", `<rss version="2.0" xmlns:nyaa="urn:nyaa"><channel><item><title>动画 01</title><link>/release/1</link><nyaa:magnetURI>` + magnet + `</nyaa:magnetURI></item></channel></rss>`, "", magnet},
		{"dmhy", "dmhy", `<html><table class="tablesorter"><tbody><tr><td>date</td><td>category</td><td><span class="tag"><a>字幕组</a></span><a href="/topics/view/1">动画 第十二集</a></td><td><a href="` + magnet + `">magnet</a></td><td>10 MB</td></tr></tbody></table></html>`, "", magnet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			manager := newBangumiTestManager(t, server.Client(), server.URL, map[string]any{"factory": tc.factory})
			browse := browseBangumiFixture(t, manager)
			require.Len(t, browse.Channels, 1)
			require.Len(t, browse.Channels[0].Episodes, 1)
			result, err := manager.Resolve(context.Background(), "fixture", browse.Channels[0].Episodes[0].ID)
			require.NoError(t, err)
			assert.Equal(t, BangumiSourceReady, result.State)
			assert.Equal(t, "torrent", result.Format)
			assert.Empty(t, result.VideoURL)
			if tc.expectedTorrent != "" {
				assert.Equal(t, server.URL+tc.expectedTorrent, result.TorrentURI)
			} else {
				assert.Equal(t, tc.expectedMagnet, result.TorrentURI)
			}
			record, err := manager.TorrentRecord("fixture", browse.Channels[0].Episodes[0].ID)
			require.NoError(t, err)
			assert.NotEmpty(t, record.OriginURL)
			assert.Equal(t, tc.expectedMagnet, record.Magnet)
			assert.EqualValues(t, 1, calls.Load(), "BT browse/resolve must not fetch an HTML playback page or dandanplay")
			publicJSON, err := common.Marshal(result)
			require.NoError(t, err)
			assert.NotContains(t, string(publicJSON), server.URL)
			assert.NotContains(t, string(publicJSON), "magnet:")
		})
	}
}

func TestBangumiSourceCookieScopeAndInjectedRedirectPolicy(t *testing.T) {
	var cdnURL string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "video/mp4")
	}))
	defer cdn.Close()
	cdnURL = cdn.URL
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "quality=1080", r.Header.Get("Cookie"))
		switch r.URL.Path {
		case "/search":
			fmt.Fprint(w, `<div class="result"><a href="/subject">条目</a></div>`)
		case "/subject":
			fmt.Fprint(w, `<div class="episodes"><a href="/play">第1集</a></div>`)
		case "/play":
			fmt.Fprint(w, `<video src="/redirect.mp4"></video>`)
		case "/redirect.mp4":
			http.Redirect(w, r, cdnURL+"/media.mp4", http.StatusFound)
		}
	}))
	defer origin.Close()
	manager := newBangumiTestManager(t, origin.Client(), origin.URL, nil)
	browse := browseBangumiFixture(t, manager)
	result, err := manager.Resolve(context.Background(), "fixture", browse.Channels[0].Episodes[0].ID)
	require.NoError(t, err)
	assert.Equal(t, cdnURL+"/media.mp4", result.VideoURL)
	assert.Empty(t, result.Headers.Get("Cookie"))
	client := origin.Client()
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		return fmt.Errorf("network policy refused redirect with PRIVATE detail")
	}
	guarded := newBangumiTestManager(t, client, origin.URL, nil)
	browse = browseBangumiFixture(t, guarded)
	result, err = guarded.Resolve(context.Background(), "fixture", browse.Channels[0].Episodes[0].ID)
	require.Error(t, err)
	assert.Equal(t, BangumiSourceFailed, result.State)
	assert.NotContains(t, result.Message, "PRIVATE")
}

func TestBangumiSourceClearanceReachesOnlyTheSourceHost(t *testing.T) {
	var cdnCookie atomic.Value
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnCookie.Store(r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "video/mp4")
	}))
	defer cdn.Close()
	var originCookie atomic.Value
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originCookie.Store(r.Header.Get("Cookie"))
		switch r.URL.Path {
		case "/search":
			fmt.Fprint(w, `<div class="result"><a href="/subject">条目</a></div>`)
		case "/subject":
			fmt.Fprint(w, `<div class="episodes"><a href="/play">第1集</a></div>`)
		case "/play":
			fmt.Fprintf(w, `<video src="%s/media.mp4"></video>`, cdn.URL)
		}
	}))
	defer origin.Close()
	manager := newBangumiTestManager(t, origin.Client(), origin.URL, nil)

	// A header-injecting or oversized value is refused rather than forwarded.
	require.Error(t, manager.SetClearance("fixture", "cf_clearance=x\r\nInjected: 1"))
	require.Error(t, manager.SetClearance("fixture", strings.Repeat("a", bangumiSourceClearanceLimit+1)))
	require.NoError(t, manager.SetClearance("fixture", "cf_clearance=abc123"))

	browse := browseBangumiFixture(t, manager)
	_, err := manager.Resolve(context.Background(), "fixture", browse.Channels[0].Episodes[0].ID)
	require.NoError(t, err)

	// The source's own host gets the imported preference and the viewer's
	// clearance together...
	sent := originCookie.Load().(string)
	assert.Contains(t, sent, "quality=1080")
	assert.Contains(t, sent, "cf_clearance=abc123")
	// ...while a media CDN on another host never sees the clearance.
	assert.Empty(t, cdnCookie.Load().(string))

	require.NoError(t, manager.SetClearance("fixture", ""))
}

func TestBangumiSourceImportedSelectorCompatibility(t *testing.T) {
	t.Run("real unquoted href prefix", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/search" {
				fmt.Fprint(w, `<div class="result"><a href="/subject">条目</a></div>`)
				return
			}
			fmt.Fprint(w, `<ul class="nav-tabs"><li><a href="#playlist1">超清A</a></li></ul><ul class="myui-content__list"><li><a href="/play">第01集</a></li></ul>`)
		}))
		defer server.Close()
		manager := newBangumiTestManager(t, server.Client(), server.URL, map[string]any{"channelFormatId": "index-grouped", "selectorChannelFormatFlattened": map[string]any{"selectChannelNames": ".nav-tabs a[href^=#playlist]", "matchChannelName": "(.*超清.*)", "selectEpisodeLists": ".myui-content__list", "selectEpisodesFromList": "a"}})
		result := browseBangumiFixture(t, manager)
		require.Len(t, result.Channels, 1)
		assert.Equal(t, "超清A", result.Channels[0].Label)
		assert.Equal(t, "01", result.Channels[0].Episodes[0].EpisodeNumber)
	})
	t.Run("all imported web definitions compile before local search", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			fmt.Fprint(w, `<html><body>No results</body></html>`)
		}))
		defer server.Close()
		target, err := url.Parse(server.URL)
		require.NoError(t, err)
		upstream := server.Client().Transport
		client := &http.Client{Transport: bangumiSourceTestTransport(func(request *http.Request) (*http.Response, error) {
			// Preserve the real catalog and route every external-boundary request
			// exclusively into this httptest server. No real site is contacted.
			local := request.Clone(request.Context())
			location := *request.URL
			location.Scheme, location.Host = target.Scheme, target.Host
			local.URL, local.Host = &location, target.Host
			response, err := upstream.RoundTrip(local)
			if response != nil {
				response.Request = request
			}
			return response, err
		})}
		manager, err := NewBangumiSourceManager(client, "")
		require.NoError(t, err)
		for _, source := range manager.Sources() {
			if source.Kind != "web" {
				continue
			}
			t.Run(source.Name, func(t *testing.T) {
				result, err := manager.Search(context.Background(), source.ID, "动漫")
				require.NoError(t, err, source.Name)
				assert.Equal(t, BangumiSourceEmpty, result.State)
			})
		}
		assert.EqualValues(t, 18, calls.Load())
	})
}

type bangumiSourceTestTransport func(*http.Request) (*http.Response, error)

func (transport bangumiSourceTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestBangumiSourceSubjectIDsSurviveResultReordering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "first" {
			fmt.Fprint(w, `<div class="result"><a href="/a">A</a><a href="/b">B</a></div>`)
		} else {
			fmt.Fprint(w, `<div class="result"><a href="/b">B</a><a href="/a">A</a></div>`)
		}
	}))
	defer server.Close()
	manager := newBangumiTestManager(t, server.Client(), server.URL, nil)
	first, err := manager.Search(context.Background(), "fixture", "first")
	require.NoError(t, err)
	second, err := manager.Search(context.Background(), "fixture", "second")
	require.NoError(t, err)
	require.Len(t, first.Subjects, 2)
	require.Len(t, second.Subjects, 2)
	assert.Equal(t, first.Subjects[0].ID, second.Subjects[1].ID)
	assert.Equal(t, first.Subjects[1].ID, second.Subjects[0].ID)
}

func TestBangumiSourceMediaProbeFallback(t *testing.T) {
	cases := []struct {
		name                                     string
		headStatus                               int
		headType, getType, prefix, state, format string
		ignoreRange                              bool
	}{
		{"HEAD 405", 405, "text/html", "video/mp4", "", BangumiSourceReady, "mp4", false},
		{"HEAD 501", 501, "", "application/vnd.apple.mpegurl", "", BangumiSourceReady, "hls", false},
		{"HEAD refused only", 403, "text/html", "video/x-matroska", "", BangumiSourceReady, "matroska", false},
		{"ignored Range bounded signature", 200, "application/octet-stream", "application/octet-stream", "\x00\x00\x00\x18ftypisom", BangumiSourceReady, "mp4", true},
		{"extension is not media evidence", 200, "application/octet-stream", "application/octet-stream", "not a media file", BangumiSourceBrowserRequired, "unknown", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probeCanceled := make(chan struct{})
			var probes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/search":
					fmt.Fprint(w, `<div class="result"><a href="/subject">条目</a></div>`)
				case "/subject":
					fmt.Fprint(w, `<div class="episodes"><a href="/play">第1集</a></div>`)
				case "/play":
					fmt.Fprint(w, `<source src="/media.mp4">`)
				case "/media.mp4":
					probes.Add(1)
					if r.Method == http.MethodHead {
						w.Header().Set("Content-Type", tc.headType)
						w.WriteHeader(tc.headStatus)
						return
					}
					assert.Equal(t, "bytes=0-0", r.Header.Get("Range"))
					w.Header().Set("Content-Type", tc.getType)
					if tc.ignoreRange {
						fmt.Fprint(w, tc.prefix+strings.Repeat("x", 1024))
						w.(http.Flusher).Flush()
						<-r.Context().Done()
						close(probeCanceled)
					} else {
						w.WriteHeader(http.StatusPartialContent)
						fmt.Fprint(w, tc.prefix)
					}
				}
			}))
			defer server.Close()
			manager := newBangumiTestManager(t, server.Client(), server.URL, nil)
			browse := browseBangumiFixture(t, manager)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := manager.Resolve(ctx, "fixture", browse.Channels[0].Episodes[0].ID)
			if tc.state == BangumiSourceReady {
				require.NoError(t, err)
				assert.NotEmpty(t, result.VideoURL)
			} else {
				require.Error(t, err)
				assert.Empty(t, result.VideoURL)
			}
			assert.Equal(t, tc.state, result.State)
			assert.Equal(t, tc.format, result.Format)
			assert.EqualValues(t, 2, probes.Load())
			if tc.ignoreRange {
				select {
				case <-probeCanceled:
				case <-ctx.Done():
					t.Fatal("ignored Range response was not canceled promptly")
				}
			}
		})
	}
}

func TestBangumiSourceFetchConcurrencyAndPacing(t *testing.T) {
	t.Run("two active upstream fetches", func(t *testing.T) {
		started := make(chan struct{}, 3)
		release := make(chan struct{})
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			started <- struct{}{}
			select {
			case <-release:
				fmt.Fprint(w, `<html>No results</html>`)
			case <-r.Context().Done():
			}
		}))
		defer server.Close()
		manager := newBangumiTestManager(t, server.Client(), server.URL, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		finished := make(chan error, 2)
		for _, keyword := range []string{"first", "second"} {
			go func() { _, err := manager.Search(ctx, "fixture", keyword); finished <- err }()
		}
		for range 2 {
			select {
			case <-started:
			case <-ctx.Done():
				close(release)
				t.Fatal("upstream fixture did not start")
			}
		}
		thirdCtx, thirdCancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer thirdCancel()
		result, err := manager.Search(thirdCtx, "fixture", "third")
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, BangumiSourceFailed, result.State)
		assert.EqualValues(t, 2, calls.Load(), "queued third request must not reach upstream")
		close(release)
		for range 2 {
			require.NoError(t, <-finished)
		}
	})
	t.Run("site interval waits are cancelable", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `<html>No results</html>`) }))
		defer server.Close()
		manager := newBangumiTestManager(t, server.Client(), server.URL, map[string]any{"requestInterval": 60000})
		_, err := manager.Search(context.Background(), "fixture", "first")
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err = manager.Search(ctx, "fixture", "second")
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.EqualValues(t, 1, calls.Load())
	})
	t.Run("zero search TTL disables reuse", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			fmt.Fprint(w, `<div class="result"><a href="/subject">条目</a></div>`)
		}))
		defer server.Close()
		manager := newBangumiTestManager(t, server.Client(), server.URL, map[string]any{"searchCacheTtl": 0})
		first, err := manager.Search(context.Background(), "fixture", "same")
		require.NoError(t, err)
		second, err := manager.Search(context.Background(), "fixture", "same")
		require.NoError(t, err)
		assert.EqualValues(t, 2, calls.Load())
		assert.Equal(t, first.Subjects[0].ID, second.Subjects[0].ID)
	})
}
