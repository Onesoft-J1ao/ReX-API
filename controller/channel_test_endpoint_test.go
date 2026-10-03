package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTestRequestForSystemOne(t *testing.T) {
	request := buildTestRequest("opencode/jev-1.13-free", string(constant.EndpointTypeSystemOne), nil, false)

	systemOne, ok := request.(*dto.SystemOneRequest)
	require.Truef(t, ok, "the System One endpoint must build a SystemOneRequest, got %T", request)
	assert.Equal(t, "opencode/jev-1.13-free", systemOne.Model)
	// The upstream rejects anything that does not validate, so a probe built
	// here must satisfy the same contract a real caller does.
	require.NoError(t, systemOne.Validate())
}

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
		{
			// testChannel wraps every failure in a localErr, so a localErr must
			// not by itself stop the walk; the upstream status decides.
			name:   "an upstream refusal still walks although it carries a local error",
			result: testResult{localErr: errors.New("bad response status code 404"), newAPIError: types.NewErrorWithStatusCode(errors.New("no such model"), types.ErrorCodeBadResponseStatusCode, http.StatusNotFound)},
			want:   true,
		},
		{
			name:   "an endpoint that cannot route the model",
			result: endpointMissing(http.StatusMethodNotAllowed, "method not allowed on this path"),
			want:   true,
		},
		{
			// An OpenCode pool answers a System One model on the chat endpoint
			// with 502 and "SystemOneRequired"; the decision endpoint serves it.
			name:   "a pool that cannot route the model on this endpoint",
			result: endpointMissing(http.StatusBadGateway, "HTTP -1 SystemOneRequired: jev is a structured decision model"),
			want:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canRetryOnAnotherEndpoint(tc.result))
		})
	}
}
