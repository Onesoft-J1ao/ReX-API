package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/stretchr/testify/assert"
)

func TestCanRetryOnAnotherEndpoint(t *testing.T) {
	// Types of upstream answers that say nothing about the endpoint, only that
	// this model is not served here.
	endpointMissing := func(status int, message string) testResult {
		return testResult{newAPIError: types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeBadResponseStatusCode, status)}
	}

	tests := []struct {
		name   string
		result testResult
		want   bool
	}{
		{
			name:   "a model the endpoint does not serve",
			result: endpointMissing(http.StatusBadRequest, "The model `deepseek-v3` does not exist or you do not have access to it"),
			want:   true,
		},
		{
			name:   "an endpoint the provider does not offer",
			result: endpointMissing(http.StatusNotFound, "unsupported endpoint"),
			want:   true,
		},
		{
			name:   "a chinese rejection",
			result: endpointMissing(http.StatusBadRequest, "该模型不支持此端点"),
			want:   true,
		},
		{
			name:   "a bad request that is not about the endpoint",
			result: endpointMissing(http.StatusBadRequest, "invalid request: prompt is too long"),
			want:   false,
		},
		{
			name:   "an auth failure repeats everywhere",
			result: endpointMissing(http.StatusUnauthorized, "invalid api key provided"),
			want:   false,
		},
		{
			name:   "a rate limit repeats everywhere",
			result: endpointMissing(http.StatusTooManyRequests, "rate limit reached for requests"),
			want:   false,
		},
		{
			name:   "an upstream outage is not an endpoint mismatch",
			result: endpointMissing(http.StatusInternalServerError, "upstream model not supported"),
			want:   false,
		},
		{
			name:   "a channel-side failure is never retried elsewhere",
			result: testResult{localErr: errors.New("model not found")},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canRetryOnAnotherEndpoint(tc.result))
		})
	}
}
