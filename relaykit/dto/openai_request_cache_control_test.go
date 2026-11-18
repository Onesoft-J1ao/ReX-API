package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageParseContentPreservesCacheControl(t *testing.T) {
	message := Message{
		Content: []any{
			map[string]any{
				"type":          "text",
				"text":          "cached prefix",
				"cache_control": map[string]any{"type": "ephemeral"},
			},
			map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": "https://example.com/image.png"},
				"cache_control": map[string]any{
					"type": "ephemeral",
					"ttl":  "1h",
				},
			},
		},
	}

	content := message.ParseContent()
	require.Len(t, content, 2)
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(content[0].CacheControl))
	assert.JSONEq(t, `{"type":"ephemeral","ttl":"1h"}`, string(content[1].CacheControl))
}
