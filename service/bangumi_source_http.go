package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
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
	"github.com/QuantumNous/new-api/setting/system_setting"
	"golang.org/x/net/proxy"
)

type bangumiDNSAnswer struct {
	addresses []net.IP
	expires   time.Time
}

type bangumiPublicTransport struct {
	proxy         *url.URL
	direct        *http.Transport
	dnsClient     *http.Client
	resolve       func(context.Context, string) ([]net.IP, error)
	getProtection func() (*common.SSRFProtection, error)
	mutex         sync.Mutex
	answers       map[string]bangumiDNSAnswer
	transports    map[string]*http.Transport
}

// Public source pages must not inherit operator-only access to private networks.
func bangumiPublicProtection() (*common.SSRFProtection, error) {
	setting := system_setting.GetFetchSetting()
	// The operator's allowlist governs which ports a source *page* may use. A
	// media CDN, though, publishes on whatever port it likes -- the same site
	// that serves HTML on 443 streams from 8088 -- so the anime fetcher admits a
	// streaming-friendly range on top of it. Blocking private addresses is what
	// keeps this from reaching internal services; a public host on a high port
	// is the normal case here, not the threat.
	ports := append(slices.Clone(setting.AllowedPorts), "8000-9999")
	return common.NewSSRFProtectionFromFetchSetting(false, setting.DomainFilterMode,
		setting.IpFilterMode, setting.DomainList, setting.IpList, ports, true)
}

func NewBangumiPublicHTTPClient(rawProxy string) (*http.Client, error) {
	proxyURL, err := common.ParseProxyURLStrict(rawProxy)
	if err != nil {
		return nil, err
	}
	if proxyURL != nil && proxyURL.Scheme == "https" {
		return nil, errors.New("public media fetching requires an HTTP or SOCKS proxy")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &bangumiPublicTransport{
		proxy: proxyURL, getProtection: bangumiPublicProtection,
		answers: make(map[string]bangumiDNSAnswer), transports: make(map[string]*http.Transport),
	}
	protectedDialer := &protectedFetchDialer{
		resolver: net.DefaultResolver, dialContext: dialer.DialContext,
		getProtection: func() (*common.SSRFProtection, bool, error) {
			protection, err := bangumiPublicProtection()
			return protection, true, err
		},
	}
	transport.direct = &http.Transport{
		Proxy: nil, DialContext: protectedDialer.DialContext,
		ForceAttemptHTTP2: true, MaxIdleConns: 32, MaxIdleConnsPerHost: 2,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	if proxyURL != nil {
		// Proxy-side DNS avoids poisoned local answers; CONNECT is pinned to a validated IP.
		transport.dnsClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL), DialContext: dialer.DialContext,
				ForceAttemptHTTP2: true, MaxIdleConns: 4, MaxIdleConnsPerHost: 2,
				IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 10 * time.Second,
			},
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &http.Client{
		Transport: transport, Timeout: 35 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("media redirect limit reached")
			}
			if len(via) > 0 && request.URL.Host != via[len(via)-1].URL.Host {
				request.Header.Del("Cookie")
				request.Header.Del("Authorization")
			}
			return nil
		},
	}, nil
}

func (t *bangumiPublicTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil ||
		(request.URL.Scheme != "https" && request.URL.Scheme != "http") || request.URL.User != nil {
		return nil, errors.New("invalid public media URL")
	}
	protection, err := t.getProtection()
	if err != nil {
		return nil, err
	}
	host := request.URL.Hostname()
	port := 443
	if request.URL.Scheme == "http" {
		port = 80
	}
	if raw := request.URL.Port(); raw != "" {
		port, err = strconv.Atoi(raw)
		if err != nil {
			return nil, errors.New("invalid media port")
		}
	}
	if err := protection.ValidateNetworkTarget(host, port); err != nil {
		return nil, err
	}
	if t.proxy == nil {
		return t.direct.RoundTrip(request)
	}

	addresses := []net.IP{net.ParseIP(host)}
	if addresses[0] == nil {
		addresses, err = t.resolvePublic(request.Context(), host)
		if err != nil {
			return nil, err
		}
	}
	var address net.IP
	for _, candidate := range addresses {
		if candidate != nil && protection.ValidateResolvedIP(host, candidate) == nil {
			address = candidate
			break
		}
	}
	if address == nil {
		return nil, errors.New("media target has no allowed public address")
	}

	pinned := request.Clone(request.Context())
	pinned.URL.Host = net.JoinHostPort(address.String(), strconv.Itoa(port))
	if pinned.Host == "" {
		pinned.Host = request.URL.Host
	}
	response, err := t.transportFor(request.URL, address).RoundTrip(pinned)
	if response != nil {
		// Relative redirects must resolve against the original name, not the pinned IP.
		response.Request = request
	}
	return response, err
}

func (t *bangumiPublicTransport) resolvePublic(ctx context.Context, host string) ([]net.IP, error) {
	if t.resolve != nil {
		return t.resolve(ctx, host)
	}
	t.mutex.Lock()
	cached, ok := t.answers[host]
	t.mutex.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.addresses, nil
	}

	var addresses []net.IP
	for _, queryType := range []string{"A", "AAAA"} {
		values := url.Values{"name": {host}, "type": {queryType}}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet,
			"https://cloudflare-dns.com/dns-query?"+values.Encode(), nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", "application/dns-json")
		response, err := t.dnsClient.Do(request)
		if err != nil {
			return nil, errors.New("proxy-side DNS lookup failed")
		}
		var answer struct {
			Status int `json:"Status"`
			Answer []struct {
				Type int    `json:"type"`
				Data string `json:"data"`
			} `json:"Answer"`
		}
		decodeErr := common.DecodeJson(io.LimitReader(response.Body, 32<<10), &answer)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || decodeErr != nil || answer.Status != 0 {
			return nil, errors.New("invalid proxy-side DNS answer")
		}
		for _, record := range answer.Answer {
			if record.Type != 1 && record.Type != 28 {
				continue
			}
			if address := net.ParseIP(record.Data); address != nil {
				addresses = append(addresses, address)
			}
		}
		if len(addresses) > 0 {
			break
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("proxy-side DNS found no address")
	}
	t.mutex.Lock()
	for key, value := range t.answers {
		if time.Now().After(value.expires) {
			delete(t.answers, key)
		}
	}
	if len(t.answers) < 128 {
		t.answers[host] = bangumiDNSAnswer{addresses: addresses, expires: time.Now().Add(time.Minute)}
	}
	t.mutex.Unlock()
	return addresses, nil
}

func (t *bangumiPublicTransport) transportFor(origin *url.URL, address net.IP) *http.Transport {
	key := origin.Scheme + "://" + strings.ToLower(origin.Host) + "/" + address.String()
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if transport := t.transports[key]; transport != nil {
		return transport
	}
	transport := &http.Transport{
		Proxy: nil, DialContext: t.dialProxy,
		ForceAttemptHTTP2: true,
		MaxIdleConns:      4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		TLSClientConfig: &tls.Config{ServerName: origin.Hostname(), MinVersion: tls.VersionTLS12},
	}
	if len(t.transports) < 64 {
		t.transports[key] = transport
	} else {
		transport.DisableKeepAlives = true
	}
	return transport
}

type bangumiTunnelConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bangumiTunnelConn) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}

// CONNECT also pins plain HTTP; an absolute proxy request can otherwise re-resolve Host.
func (t *bangumiPublicTransport) dialProxy(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if t.proxy.Scheme == "socks5" || t.proxy.Scheme == "socks5h" {
		config := *t.proxy
		config.Scheme = "socks5"
		socks, err := proxy.FromURL(&config, dialer)
		if err != nil {
			return nil, err
		}
		contextDialer, ok := socks.(proxy.ContextDialer)
		if !ok {
			return nil, errors.New("proxy does not support contextual dialing")
		}
		return contextDialer.DialContext(ctx, network, address)
	}

	proxyAddress := t.proxy.Host
	if t.proxy.Port() == "" {
		proxyAddress = net.JoinHostPort(t.proxy.Hostname(), "80")
	}
	connection, err := dialer.DialContext(ctx, network, proxyAddress)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(10 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := connection.SetDeadline(deadline); err != nil {
		connection.Close()
		return nil, err
	}
	stopCancel := context.AfterFunc(ctx, func() { connection.Close() })
	defer stopCancel()
	request := &http.Request{
		Method: http.MethodConnect, URL: &url.URL{Opaque: address},
		Host: address, Header: make(http.Header),
	}
	if t.proxy.User != nil {
		password, _ := t.proxy.User.Password()
		credentials := t.proxy.User.Username() + ":" + password
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(credentials)))
	}
	if err := request.Write(connection); err != nil {
		connection.Close()
		return nil, err
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		connection.Close()
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		connection.Close()
		return nil, errors.New("proxy CONNECT rejected")
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		connection.Close()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		connection.Close()
		return nil, err
	}
	return &bangumiTunnelConn{Conn: connection, reader: reader}, nil
}

func (t *bangumiPublicTransport) CloseIdleConnections() {
	t.direct.CloseIdleConnections()
	if t.dnsClient != nil {
		t.dnsClient.CloseIdleConnections()
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()
	for _, transport := range t.transports {
		transport.CloseIdleConnections()
	}
}
