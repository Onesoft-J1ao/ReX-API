package channel

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpstreamFailoverDoesNotReplayAResponseAlreadyReceived(t *testing.T) {
	assert.True(t, upstreamFailoverWorthy(nil, assert.AnError))
	assert.True(t, upstreamFailoverWorthy(&http.Response{StatusCode: http.StatusTooManyRequests}, nil))
	assert.False(t, upstreamFailoverWorthy(&http.Response{StatusCode: http.StatusOK}, assert.AnError))
	assert.False(t, upstreamFailoverWorthy(&http.Response{StatusCode: http.StatusBadRequest}, nil))
	assert.False(t, upstreamFailoverWorthy(nil, nil))
}

func TestAlternateUpstreamURLsKeepsThePathAndSkipsOtherHosts(t *testing.T) {
	// A channel on the primary root falls back to the mirror, keeping the path
	// under /v1 intact.
	assert.Equal(t, []string{
		"https://gs.mfawa.top/v1/chat/completions",
	}, alternateUpstreamURLs("https://api.furry.vg/v1/chat/completions"))

	// A channel on the mirror prefers the primary root.
	assert.Equal(t, []string{
		"https://api.furry.vg/v1/chat/completions",
	}, alternateUpstreamURLs("https://gs.mfawa.top/v1/chat/completions"))

	assert.Empty(t, alternateUpstreamURLs("https://api.openai.com/v1/chat/completions"))
}
