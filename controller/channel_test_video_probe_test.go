/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The probe must never create a render task — a real submit would cost money on
// every probe cycle — so it leans on the upstream's own rejection of a
// prompt-less request. These cases pin the three answers it has to tell apart.
func TestProbeVideoEndpointReachable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// GetHttpClient is nil until the service initialises it, and the probe
	// reaches the upstream through it.
	service.InitHttpClient()

	tests := []struct {
		name        string
		status      int
		body        string
		wantSuccess bool
	}{
		{
			name:        "the endpoint read the request and rejected the missing prompt",
			status:      http.StatusBadRequest,
			body:        `{"code":"invalid_request","message":"prompt is required"}`,
			wantSuccess: true,
		},
		{
			name:   "the endpoint does not exist",
			status: http.StatusNotFound,
			body:   `{"error":{"message":"Invalid URL (POST /v1/videos)"}}`,
		},
		{
			name:   "the upstream has no channel for the model",
			status: http.StatusServiceUnavailable,
			body:   `{"error":{"code":"model_not_found","message":"无可用渠道"}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var probedPaths []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				probedPaths = append(probedPaths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()

			baseURL := upstream.URL
			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Key:     "nvapi-test",
				BaseURL: &baseURL,
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

			result := probeVideoEndpointReachable(c, channel, "agnes-video-v2.0")

			require.Equal(t, []string{"/v1/videos"}, probedPaths,
				"the probe must hit the video endpoint and nothing else")
			if tc.wantSuccess {
				assert.NoError(t, result.localErr)
				assert.Nil(t, result.newAPIError)
				return
			}
			assert.Error(t, result.localErr)
			assert.NotNil(t, result.newAPIError)
		})
	}
}

// A channel whose every key is dead must fail rather than report the endpoint
// as reachable off the back of one key that happened to answer.
func TestProbeVideoEndpointTriesEveryKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var seen int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"prompt is required"}`))
	}))
	defer upstream.Close()

	baseURL := upstream.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "key-a" + "\n" + "key-b",
		BaseURL: &baseURL,
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	result := probeVideoEndpointReachable(c, channel, "agnes-video-v2.0")

	assert.NoError(t, result.localErr)
	assert.Equal(t, 1, seen, "a reachable endpoint must not be probed key by key")
}

func TestProbeVideoEndpointFailsWithoutKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)

	baseURL := "http://127.0.0.1:1"
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, BaseURL: &baseURL}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	result := probeVideoEndpointReachable(c, channel, "agnes-video-v2.0")

	require.Error(t, result.localErr)
	assert.Contains(t, result.localErr.Error(), "no usable key")
}
