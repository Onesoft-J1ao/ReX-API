package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/proxy_pool_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProxyHostPort(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "bare host:port as the pool returns it", raw: "1.2.3.4:8888", want: "1.2.3.4:8888"},
		{name: "scheme prefix is stripped", raw: "http://1.2.3.4:8888", want: "1.2.3.4:8888"},
		{name: "socks5 prefix is stripped", raw: "socks5://1.2.3.4:1080", want: "1.2.3.4:1080"},
		{name: "surrounding whitespace is trimmed", raw: " 1.2.3.4:8888\n", want: "1.2.3.4:8888"},
		{name: "hostname is accepted", raw: "proxy.example.com:3128", want: "proxy.example.com:3128"},
		{name: "missing port", raw: "1.2.3.4", wantErr: true},
		{name: "port zero", raw: "1.2.3.4:0", wantErr: true},
		{name: "port out of range", raw: "1.2.3.4:70000", wantErr: true},
		{name: "non numeric port", raw: "1.2.3.4:abc", wantErr: true},
		{name: "a url with a path is not a proxy", raw: "http://evil.example.com/path", wantErr: true},
		{name: "embedded whitespace", raw: "1.2.3.4 :8888", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
		{name: "empty host", raw: ":8888", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateProxyHostPort(tc.raw)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseSingleProxy(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{name: "documented json shape", body: `{"proxy": "1.2.3.4:8888"}`, want: "1.2.3.4:8888"},
		{name: "json data field", body: `{"data": "1.2.3.4:8888"}`, want: "1.2.3.4:8888"},
		{name: "plain text as this deployment returns", body: "1.2.3.4:8888\n", want: "1.2.3.4:8888"},
		{name: "json without proxy field", body: `{"count": 3}`, wantErr: true},
		{name: "malformed json", body: `{"proxy": `, wantErr: true},
		{name: "empty", body: "  ", wantErr: true},
		{name: "html error page", body: "<html>404</html>", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSingleProxy([]byte(tc.body))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseProxyList(t *testing.T) {
	t.Run("plain text one per line", func(t *testing.T) {
		proxies, err := parseProxyList([]byte("115.29.149.2:9999\n210.77.11.248:10808\n8.213.222.157:443\n"))
		require.NoError(t, err)
		assert.Equal(t, []string{"115.29.149.2:9999", "210.77.11.248:10808", "8.213.222.157:443"}, proxies)
	})

	t.Run("invalid lines are skipped", func(t *testing.T) {
		proxies, err := parseProxyList([]byte("1.2.3.4:8888\nnot-a-proxy\n\n5.6.7.8:3128"))
		require.NoError(t, err)
		assert.Equal(t, []string{"1.2.3.4:8888", "5.6.7.8:3128"}, proxies)
	})

	t.Run("json array", func(t *testing.T) {
		proxies, err := parseProxyList([]byte(`["1.2.3.4:8888", "5.6.7.8:3128"]`))
		require.NoError(t, err)
		assert.Equal(t, []string{"1.2.3.4:8888", "5.6.7.8:3128"}, proxies)
	})

	t.Run("duplicates collapse", func(t *testing.T) {
		proxies, err := parseProxyList([]byte("1.2.3.4:8888\n1.2.3.4:8888\n"))
		require.NoError(t, err)
		assert.Equal(t, []string{"1.2.3.4:8888"}, proxies)
	})

	t.Run("empty body is an empty pool", func(t *testing.T) {
		proxies, err := parseProxyList([]byte("   \n"))
		require.NoError(t, err)
		assert.Empty(t, proxies)
	})

	// A 200 response that is not a proxy list means the format changed; reporting
	// "zero proxies" here would silently look like an empty pool.
	t.Run("html body is an error, not an empty pool", func(t *testing.T) {
		_, err := parseProxyList([]byte("<html><body>Proxy Pool</body></html>"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no valid host:port")
	})
}

func TestParseCount(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    int
		wantErr bool
	}{
		{name: "plain integer as this deployment returns", body: "1170", want: 1170},
		{name: "plain integer with whitespace", body: " 1170\n", want: 1170},
		{name: "json object", body: `{"count": 42}`, want: 42},
		{name: "zero", body: "0", want: 0},
		{name: "html error page", body: "<html>500</html>", wantErr: true},
		{name: "empty", body: "", wantErr: true},
		{name: "malformed json", body: `{"count": `, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCount([]byte(tc.body))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// usePoolServer points the client at a test server through the same option-load
// path production uses, and restores the previous URL when the test ends.
func usePoolServer(t *testing.T, serverURL string) {
	t.Helper()
	previous := proxy_pool_setting.GetSetting().PoolURL
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"proxy_pool_setting.pool_url": previous,
		}))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"proxy_pool_setting.pool_url": serverURL,
	}))
	require.Equal(t, serverURL, proxy_pool_setting.GetPoolURL())
}

func TestProxyPoolClientAgainstDeploymentDeviations(t *testing.T) {
	var singleHits, randomHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/count":
			_, _ = w.Write([]byte("1170"))
		case "/all":
			_, _ = w.Write([]byte("115.29.149.2:9999\n210.77.11.248:10808\n"))
		case "/get":
			// The live pool 404s /get even though the API doc lists it.
			singleHits++
			http.NotFound(w, r)
		case "/random":
			randomHits++
			_, _ = w.Write([]byte("47.104.27.165:8443"))
		case "/delete":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	usePoolServer(t, server.URL)

	client, err := NewProxyPoolClient()
	require.NoError(t, err)
	ctx := context.Background()

	count, err := client.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1170, count)

	proxies, err := client.GetAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"115.29.149.2:9999", "210.77.11.248:10808"}, proxies)

	// GetOne must fall back from the documented /get to the /random route this
	// deployment actually serves, and must remember the working route.
	proxy, err := client.GetOne(ctx)
	require.NoError(t, err)
	assert.Equal(t, "47.104.27.165:8443", proxy)

	proxy, err = client.GetOne(ctx)
	require.NoError(t, err)
	assert.Equal(t, "47.104.27.165:8443", proxy)
	assert.Equal(t, 1, singleHits, "the /get probe must only be paid once")
	assert.Equal(t, 2, randomHits)

	// Delete must report the unsupported route rather than pretending success.
	err = client.Delete(ctx, "1.2.3.4:8888")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not expose /delete")

	// A malformed proxy must be rejected before any request is made.
	err = client.Delete(ctx, "not a proxy")
	require.Error(t, err)
}

func TestProxyPoolClientRejectsBadPoolURL(t *testing.T) {
	usePoolServer(t, "")
	_, err := NewProxyPoolClient()
	require.Error(t, err)

	usePoolServer(t, "ftp://pool.example.com")
	_, err = NewProxyPoolClient()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be http")
}

func TestProxyPoolClientBoundsResponseSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, strings.Repeat("1.2.3.4:8888\n", maxProxyPoolResponseBytes/12+16))
	}))
	defer server.Close()
	usePoolServer(t, server.URL)

	client, err := NewProxyPoolClient()
	require.NoError(t, err)
	_, err = client.GetAll(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestProxyPoolClientSurfacesHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	usePoolServer(t, server.URL)

	client, err := NewProxyPoolClient()
	require.NoError(t, err)
	_, err = client.Count(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "http 500")
}
