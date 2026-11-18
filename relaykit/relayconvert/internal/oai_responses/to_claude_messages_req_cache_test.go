package oairesponses

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesRequestToClaudeMessagesPreservesExplicitCacheBreakpoints(t *testing.T) {
	maxTokens := uint(128)
	meta := &convmeta.Values{Options: &convmeta.Options{
		Claude: convmeta.ClaudeOptions{ResponsesPromptCaching: true},
	}}
	request := &dto.OpenAIResponsesRequest{
		Model:           "claude-test",
		MaxOutputTokens: &maxTokens,
		Input: mustRawMessage(t, []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{
						"type":                   "input_text",
						"text":                   "cache this stable prefix",
						"prompt_cache_breakpoint": map[string]any{"mode": "explicit"},
					},
				},
			},
		}),
	}

	got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), meta, request)
	require.NoError(t, err)
	messages, ok := got.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, messages, 1)
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(messages[0].CacheControl))
	assert.Empty(t, got.CacheControl, "one explicit breakpoint should not add another implicit breakpoint")
}

func TestResponsesRequestToClaudeMessagesKeepsCachingOptIn(t *testing.T) {
	maxTokens := uint(128)
	request := &dto.OpenAIResponsesRequest{
		Model:           "claude-test",
		MaxOutputTokens: &maxTokens,
		PromptCacheOptions: mustRawMessage(t, map[string]any{
			"mode": "implicit",
			"ttl":  "30m",
		}),
		Input: mustRawMessage(t, []any{
			map[string]any{"type": "message", "role": "user", "content": "hello"},
			map[string]any{"type": "message", "role": "assistant", "content": "hi"},
		}),
	}

	got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), &convmeta.Values{}, request)
	require.NoError(t, err)
	assert.Empty(t, got.CacheControl)
	parts := got.Messages[0].Content.([]dto.ClaudeMediaMessage)
	assert.Empty(t, parts[0].CacheControl)
}

func TestResponsesRequestToClaudeMessagesAppliesImplicitCacheOnlyWhenRequested(t *testing.T) {
	maxTokens := uint(128)
	request := &dto.OpenAIResponsesRequest{
		Model:              "claude-test",
		MaxOutputTokens:    &maxTokens,
		PromptCacheOptions: mustRawMessage(t, map[string]any{"mode": "implicit"}),
		Input: mustRawMessage(t, []any{
			map[string]any{"type": "message", "role": "user", "content": "stable"},
			map[string]any{"type": "message", "role": "assistant", "content": "continue"},
		}),
	}
	meta := &convmeta.Values{Options: &convmeta.Options{
		Claude: convmeta.ClaudeOptions{ResponsesPromptCaching: true},
	}}

	got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), meta, request)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(got.CacheControl))
}

func TestResponsesRequestToClaudeMessagesRespectsClaudeBreakpointLimit(t *testing.T) {
	maxTokens := uint(128)
	parts := make([]any, 5)
	for i := range parts {
		parts[i] = map[string]any{
			"type":                   "input_text",
			"text":                   "breakpoint",
			"prompt_cache_breakpoint": map[string]any{"mode": "explicit"},
		}
	}
	meta := &convmeta.Values{Options: &convmeta.Options{
		Claude: convmeta.ClaudeOptions{ResponsesPromptCaching: true},
	}}
	got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), meta, &dto.OpenAIResponsesRequest{
		Model:           "claude-test",
		MaxOutputTokens: &maxTokens,
		Input: mustRawMessage(t, []any{
			map[string]any{"type": "message", "role": "user", "content": parts},
		}),
	})
	require.NoError(t, err)
	content, ok := got.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, content, 5)
	for i := range content {
		if i < 4 {
			assert.JSONEq(t, `{"type":"ephemeral"}`, string(content[i].CacheControl))
		} else {
			assert.Empty(t, content[i].CacheControl)
		}
	}
}
