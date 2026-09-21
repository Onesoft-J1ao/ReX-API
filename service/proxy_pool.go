package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/proxy_pool_setting"
)

const (
	// maxProxyPoolResponseBytes bounds every body read from the pool. /all on a
	// large pool is a long plain-text list; the cap keeps a hostile endpoint from
	// streaming unbounded data into memory.
	maxProxyPoolResponseBytes = 1 << 20
	proxyPoolUserAgent        = "new-api-proxy-pool/1.0"
)

// errProxyPoolRouteMissing marks a 404 so the client can try an alternative
// endpoint instead of failing the whole call.
var errProxyPoolRouteMissing = errors.New("proxy pool: endpoint not found")

// ProxyPoolClient speaks the jhao104/proxy_pool HTTP API. Upstream versions
// disagree on route names (this deployment serves /random where the docs say
// /get, and exposes no delete route at all), so the client probes for the
// documented route first and remembers what actually answered.
type ProxyPoolClient struct {
	baseURL    string
	proxyType  string
	httpClient *http.Client

	mu sync.Mutex
	// singlePath remembers the endpoint that served a single proxy, so only the
	// first call pays for the 404 probe.
	singlePath string
}

// NewProxyPoolClient builds a client from the current settings. It fails rather
// than silently pointing at a default host when the URL is missing or malformed.
func NewProxyPoolClient() (*ProxyPoolClient, error) {
	baseURL := proxy_pool_setting.GetPoolURL()
	if baseURL == "" {
		return nil, errors.New("proxy pool: pool_url is not configured")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("proxy pool: invalid pool_url %q: %w", baseURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("proxy pool: pool_url must be http(s), got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("proxy pool: pool_url %q has no host", baseURL)
	}
	return &ProxyPoolClient{
		baseURL:    baseURL,
		proxyType:  strings.TrimSpace(proxy_pool_setting.GetSetting().ProxyType),
		httpClient: &http.Client{Timeout: time.Duration(proxy_pool_setting.GetRequestTimeoutSeconds()) * time.Second},
	}, nil
}

func (c *ProxyPoolClient) typeQuery() url.Values {
	query := url.Values{}
	if c.proxyType != "" {
		query.Set("type", c.proxyType)
	}
	return query
}

func (c *ProxyPoolClient) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	endpoint := c.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("proxy pool: building request for %s: %w", path, err)
	}
	req.Header.Set("User-Agent", proxyPoolUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxy pool: requesting %s: %w", path, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("proxy pool: %s: %w", path, errProxyPoolRouteMissing)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy pool: %s returned http %d", path, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxProxyPoolResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("proxy pool: reading %s: %w", path, err)
	}
	if len(data) > maxProxyPoolResponseBytes {
		return nil, fmt.Errorf("proxy pool: response from %s exceeds %d bytes", path, maxProxyPoolResponseBytes)
	}
	return data, nil
}

// GetOne returns one random proxy as host:port.
func (c *ProxyPoolClient) GetOne(ctx context.Context) (string, error) {
	c.mu.Lock()
	seen := c.singlePath
	c.mu.Unlock()

	paths := []string{"/get", "/random"}
	if seen != "" {
		paths = []string{seen}
	}
	for _, path := range paths {
		data, err := c.get(ctx, path, c.typeQuery())
		if errors.Is(err, errProxyPoolRouteMissing) {
			continue
		}
		if err != nil {
			return "", err
		}
		proxy, err := parseSingleProxy(data)
		if err != nil {
			return "", fmt.Errorf("proxy pool: %s: %w", path, err)
		}
		c.mu.Lock()
		c.singlePath = path
		c.mu.Unlock()
		return proxy, nil
	}
	return "", fmt.Errorf("proxy pool: no single-proxy endpoint responded (tried %s)", strings.Join(paths, ", "))
}

// GetAll returns every proxy the pool currently holds.
func (c *ProxyPoolClient) GetAll(ctx context.Context) ([]string, error) {
	data, err := c.get(ctx, "/all", c.typeQuery())
	if err != nil {
		return nil, err
	}
	return parseProxyList(data)
}

// Count returns the number of proxies the pool currently holds.
func (c *ProxyPoolClient) Count(ctx context.Context) (int, error) {
	data, err := c.get(ctx, "/count", nil)
	if err != nil {
		return 0, err
	}
	return parseCount(data)
}

// Delete removes one proxy from the pool, used to feed back a proxy that failed
// in production. A pool that does not expose the route is reported as such
// instead of pretending the delete succeeded.
func (c *ProxyPoolClient) Delete(ctx context.Context, proxy string) error {
	normalized, err := validateProxyHostPort(proxy)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("proxy", normalized)
	if _, err := c.get(ctx, "/delete", query); err != nil {
		if errors.Is(err, errProxyPoolRouteMissing) {
			return fmt.Errorf("proxy pool: this pool does not expose /delete; cannot remove %s remotely", normalized)
		}
		return err
	}
	return nil
}

// parseSingleProxy accepts both shapes the pool has been observed to return for
// one proxy: a JSON object ({"proxy": "1.2.3.4:8888"}) and a bare plain-text line.
func parseSingleProxy(data []byte) (string, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "", errors.New("empty response")
	}
	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			Proxy string `json:"proxy"`
			Data  string `json:"data"`
		}
		if err := common.Unmarshal([]byte(trimmed), &payload); err != nil {
			return "", fmt.Errorf("malformed json: %w", err)
		}
		candidate := payload.Proxy
		if candidate == "" {
			candidate = payload.Data
		}
		if candidate == "" {
			return "", errors.New("json response carried no proxy field")
		}
		return validateProxyHostPort(candidate)
	}
	return validateProxyHostPort(trimmed)
}

// parseProxyList accepts the plain-text, one-per-line shape this deployment
// returns (/all) and a JSON array of strings. Lines that are not valid host:port
// entries are skipped; a non-empty body that yields nothing valid is an error,
// because that means the format changed rather than that the pool is empty.
func parseProxyList(data []byte) ([]string, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return []string{}, nil
	}
	proxies := []string{}
	invalid := 0
	if strings.HasPrefix(trimmed, "[") {
		var items []string
		if err := common.Unmarshal([]byte(trimmed), &items); err != nil {
			return nil, fmt.Errorf("malformed json list: %w", err)
		}
		for _, item := range items {
			normalized, err := validateProxyHostPort(item)
			if err != nil {
				invalid++
				continue
			}
			proxies = append(proxies, normalized)
		}
	} else {
		for line := range strings.SplitSeq(trimmed, "\n") {
			normalized, err := validateProxyHostPort(line)
			if err != nil {
				if strings.TrimSpace(line) != "" {
					invalid++
				}
				continue
			}
			proxies = append(proxies, normalized)
		}
	}
	if len(proxies) == 0 && invalid > 0 {
		return nil, fmt.Errorf("response contained no valid host:port proxy (%d unparsable entries)", invalid)
	}
	return slices.Compact(proxies), nil
}

func parseCount(data []byte) (int, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0, errors.New("empty response")
	}
	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			Count int `json:"count"`
		}
		if err := common.Unmarshal([]byte(trimmed), &payload); err != nil {
			return 0, fmt.Errorf("malformed json: %w", err)
		}
		return payload.Count, nil
	}
	count, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("malformed count %q: %w", trimmed, err)
	}
	return count, nil
}

// validateProxyHostPort is the trust boundary for pool output: a proxy string
// becomes a channel's outbound proxy, so anything that is not a bare host:port
// (or a scheme-prefixed host:port) with a usable port is rejected.
func validateProxyHostPort(raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return "", errors.New("empty proxy")
	}
	for _, scheme := range []string{"http://", "https://", "socks5://", "socks5h://", "socks4://"} {
		if after, ok := strings.CutPrefix(candidate, scheme); ok {
			candidate = after
			break
		}
	}
	if strings.ContainsAny(candidate, "/ \t\r") {
		return "", fmt.Errorf("proxy %q is not a bare host:port", raw)
	}
	host, port, err := net.SplitHostPort(candidate)
	if err != nil {
		return "", fmt.Errorf("proxy %q is not host:port: %w", raw, err)
	}
	if host == "" {
		return "", fmt.Errorf("proxy %q has an empty host", raw)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("proxy %q has an invalid port", raw)
	}
	return candidate, nil
}

// ------------------- relay-time auto-assignment -------------------

// poolProxyCache holds one drawn proxy. Refreshing is done off the request path:
// an AI relay request must never wait on a third-party pool, so a cold cache
// returns "no proxy" and refills in the background.
var poolProxyCache struct {
	sync.Mutex
	proxy      string
	expiresAt  time.Time
	refreshing bool
}

// PoolProxyForRelay returns a pool proxy for outbound relay traffic, or "" when
// auto-assignment is off, no proxy is cached yet, or the pool is unreachable.
// Callers treat "" as "connect directly".
func PoolProxyForRelay() string {
	cfg := proxy_pool_setting.GetSetting()
	if !cfg.Enabled || !cfg.AutoAssignChannels {
		return ""
	}
	now := time.Now()
	poolProxyCache.Lock()
	if poolProxyCache.proxy != "" && now.Before(poolProxyCache.expiresAt) {
		proxy := poolProxyCache.proxy
		poolProxyCache.Unlock()
		return proxy
	}
	if poolProxyCache.refreshing {
		poolProxyCache.Unlock()
		return ""
	}
	poolProxyCache.refreshing = true
	poolProxyCache.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(),
			time.Duration(proxy_pool_setting.GetRequestTimeoutSeconds())*time.Second)
		defer cancel()

		var drawn string
		client, err := NewProxyPoolClient()
		if err == nil {
			drawn, err = client.GetOne(ctx)
		}

		poolProxyCache.Lock()
		defer poolProxyCache.Unlock()
		poolProxyCache.refreshing = false
		if err != nil {
			common.SysLog(fmt.Sprintf("proxy pool: background refresh failed: %v", err))
			return
		}
		poolProxyCache.proxy = drawn
		poolProxyCache.expiresAt = time.Now().Add(time.Duration(proxy_pool_setting.GetCacheTTLSeconds()) * time.Second)
	}()
	return ""
}

// ForgetPoolProxy drops the cached proxy so the next refresh draws a new one.
// Used after a pooled proxy is reported dead.
func ForgetPoolProxy() {
	poolProxyCache.Lock()
	defer poolProxyCache.Unlock()
	poolProxyCache.proxy = ""
	poolProxyCache.expiresAt = time.Time{}
}
