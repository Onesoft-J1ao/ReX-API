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
	request := buildTestRequest("opencode/jev-1.13-free", string(constant.EndpointTypeSystemOne), 0, false)

	systemOne, ok := request.(*dto.SystemOneRequest)
	require.Truef(t, ok, "the System One endpoint must build a SystemOneRequest, got %T", request)
	assert.Equal(t, "opencode/jev-1.13-free", systemOne.Model)
	// The upstream rejects anything that does not validate, so a probe built
	// here must satisfy the same contract a real caller does.
	require.NoError(t, systemOne.Validate())
}

func TestVideoModelsAreClassifiedAndRouted(t *testing.T) {
	for _, name := range []string{"agnes-video-v2.0", "agnes-video-2.5-flash"} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, channelTestModelVideo,
				classifyChannelTestModel(constant.ChannelTypeOpenAI, name))
			assert.Equal(t, "/v1/videos",
				autoDetectChannelTestRequestPath(constant.ChannelTypeOpenAI, name))
		})
	}

	// A substring of "video" is not enough: this one is a detector, and sending
	// it to /v1/videos would report a failure that means nothing.
	t.Run("a detector is not a video model", func(t *testing.T) {
		assert.False(t, isVideoGenerationTestModel("nvidia/ai-synthetic-video-detector"))
		assert.Equal(t, channelTestModelChat,
			classifyChannelTestModel(constant.ChannelTypeOpenAI, "nvidia/ai-synthetic-video-detector"))
	})
}

func TestChannelTestPathAndBodyAgree(t *testing.T) {
	// The auto-detected path and the auto-detected body must classify a model
	// the same way. They used to be decided separately: a name matching "embed"
	// but not "embedding" went to /v1/embeddings with a chat body, and a
	// "bge-reranker" went to /v1/embeddings with a rerank body.
	tests := []struct {
		name     string
		model    string
		wantPath string
		wantBody any
	}{
		{name: "nvidia embedding", model: "nvidia/embed-qa-4", wantPath: "/v1/embeddings", wantBody: &dto.EmbeddingRequest{}},
		{name: "openai embedding", model: "text-embedding-3-large", wantPath: "/v1/embeddings", wantBody: &dto.EmbeddingRequest{}},
		{name: "bge embedding", model: "BAAI/bge-m3", wantPath: "/v1/embeddings", wantBody: &dto.EmbeddingRequest{}},
		{name: "bge reranker stays rerank", model: "bge-reranker-v2-m3", wantPath: "/v1/rerank", wantBody: &dto.RerankRequest{}},
		{name: "image generation", model: "gpt-image-1", wantPath: "/v1/images/generations", wantBody: &dto.ImageRequest{}},
		{name: "responses only", model: "gpt-5.3-codex", wantPath: "/v1/responses", wantBody: &dto.OpenAIResponsesRequest{}},
		{name: "plain chat", model: "gpt-6-astra", wantPath: "/v1/chat/completions", wantBody: &dto.GeneralOpenAIRequest{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantPath, autoDetectChannelTestRequestPath(constant.ChannelTypeOpenAI, tc.model))
			assert.IsType(t, tc.wantBody, buildTestRequest(tc.model, "", constant.ChannelTypeOpenAI, false))
		})
	}
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
