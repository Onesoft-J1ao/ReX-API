package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	common2 "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withFirstByteTimeout(t *testing.T, seconds int) {
	t.Helper()
	previous := common2.RelayStreamFirstByteTimeout
	common2.RelayStreamFirstByteTimeout = seconds
	t.Cleanup(func() { common2.RelayStreamFirstByteTimeout = previous })
}

// A streaming upstream that accepts the connection and never answers is the
// shape Cloudflare reports as a 524. The origin must give up first, below the
// ~100s Cloudflare budget, instead of holding the request open.
func TestStreamFirstByteDeadlineAbortsStalledUpstream(t *testing.T) {
	withFirstByteTimeout(t, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})

	req, err := http.NewRequest(http.MethodPost, server.URL, nil)
	require.NoError(t, err)
	req, deadline := wrapStreamFirstByteDeadline(req.WithContext(context.Background()), true)
	require.NotNil(t, deadline)

	start := time.Now()
	resp, err := server.Client().Do(req)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, 5*time.Second, "the request must fail at the deadline, not when the upstream finally answers")
	assert.True(t, deadline.timedOut.Load(), "the failure must be attributed to the first-byte deadline")
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	deadline.release()
}

// The whole point of using a first-byte deadline instead of http.Client.Timeout
// is that the deadline stops applying once the headers are in: a slow but
// healthy SSE stream must still run to completion.
func TestStreamFirstByteDeadlineDoesNotTruncateStreamBody(t *testing.T) {
	withFirstByteTimeout(t, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flush(w)
		for _, chunk := range []string{"first", "second", "third"} {
			time.Sleep(1500 * time.Millisecond)
			if _, err := io.WriteString(w, chunk+"\n"); err != nil {
				return
			}
			flush(w)
		}
	}))
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodPost, server.URL, nil)
	require.NoError(t, err)
	req, deadline := wrapStreamFirstByteDeadline(req, true)
	require.NotNil(t, deadline)

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	resp.Body = &firstByteTimeoutBody{rc: resp.Body, release: deadline.release}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.False(t, deadline.timedOut.Load(), "headers arrived in time, so the deadline must have been disarmed")
	assert.Equal(t, "first\nsecond\nthird\n", string(body),
		"a body that outlives the first-byte bound must not be truncated")
}

func TestStreamFirstByteDeadlineSkipsNonStreamingRequests(t *testing.T) {
	withFirstByteTimeout(t, 1)
	req, err := http.NewRequest(http.MethodPost, "http://example.invalid", nil)
	require.NoError(t, err)

	wrapped, deadline := wrapStreamFirstByteDeadline(req, false)
	assert.Nil(t, deadline)
	assert.Same(t, req, wrapped)
}

func TestStreamFirstByteDeadlineDisabledWhenUnset(t *testing.T) {
	for _, seconds := range []int{0, -1} {
		withFirstByteTimeout(t, seconds)
		req, err := http.NewRequest(http.MethodPost, "http://example.invalid", nil)
		require.NoError(t, err)

		wrapped, deadline := wrapStreamFirstByteDeadline(req, true)
		assert.Nil(t, deadline)
		assert.Same(t, req, wrapped)
	}
}

func TestFirstByteTimeoutBodyReleasesOnClose(t *testing.T) {
	released := 0
	body := &firstByteTimeoutBody{
		rc:      io.NopCloser(strings.NewReader("payload")),
		release: func() { released++ },
	}

	buf := make([]byte, 1)
	n, err := body.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	assert.Zero(t, released, "the deadline must stay armed while the body is open")

	rest, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "ayload", string(rest))
	assert.Zero(t, released)

	require.NoError(t, body.Close())
	assert.Equal(t, 1, released)
}

func TestFirstByteDeadlineReleaseIsNilSafe(t *testing.T) {
	var deadline *firstByteDeadline
	assert.NotPanics(t, deadline.release)
}

func flush(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
