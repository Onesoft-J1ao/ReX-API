package channel

import (
	"net/http"
	"net/url"
	"strings"
)

// upstreamBaseURLs are the roots one upstream publishes. A channel pointed at
// any of them retries the others when that root fails, so one host being down
// or rate-limiting does not fail the request.
var upstreamBaseURLs = []string{
	"https://api.furry.vg/v1",
	"https://api.furry.vg",
	"https://gs.mfawa.top/v1",
}

// alternateUpstreamURLs returns the same request against the other roots.
// A URL that is not under one of the roots returns nothing, so a channel
// pointed somewhere else is left alone.
func alternateUpstreamURLs(full string) []string {
	trimmed := strings.TrimRight(full, "/")
	matched := ""
	for _, base := range upstreamBaseURLs {
		if trimmed == base || strings.HasPrefix(trimmed, base+"/") {
			if len(base) > len(matched) {
				matched = base
			}
		}
	}
	if matched == "" {
		return nil
	}
	suffix := strings.TrimPrefix(trimmed, matched)
	out := make([]string, 0, len(upstreamBaseURLs)-1)
	for _, base := range upstreamBaseURLs {
		if base == matched {
			continue
		}
		out = append(out, base+suffix)
	}
	return out
}

// A transport failure (no response at all) is safe to replay. Once a response
// exists, only the statuses that mean "this root refused or never handled it"
// are. A 200 that then errors while the body is read has already been served,
// and replaying it would bill the request twice.
func upstreamFailoverWorthy(resp *http.Response, err error) bool {
	if resp == nil {
		return err != nil
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// failoverUpstream resends a failed request to the other roots. The body has to
// be rewindable; a request that cannot be replayed keeps the first result.
func failoverUpstream(req *http.Request, fullURL string, resp *http.Response, reqErr error, send func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	if !upstreamFailoverWorthy(resp, reqErr) || req.GetBody == nil {
		return resp, reqErr
	}
	alts := alternateUpstreamURLs(fullURL)
	if len(alts) == 0 {
		return resp, reqErr
	}
	for _, alt := range alts {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
			resp = nil
		}
		body, err := req.GetBody()
		if err != nil {
			return resp, reqErr
		}
		parsed, err := url.Parse(alt)
		if err != nil {
			continue
		}
		cloned := req.Clone(req.Context())
		cloned.URL = parsed
		cloned.Host = parsed.Host
		cloned.Body = body
		resp, reqErr = send(cloned)
		if !upstreamFailoverWorthy(resp, reqErr) {
			return resp, reqErr
		}
	}
	return resp, reqErr
}
