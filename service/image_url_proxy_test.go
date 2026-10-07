/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/

package service

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The artifact id is the only thing protecting the image, and the browser
// fetches it from an <img> that cannot carry an Authorization header. The id
// must therefore be unguessable and independent of the request id the caller
// already holds, while each URL in one response still gets its own entry.
func TestRewriteGeneratedImageURLsUsesConfiguredAbsoluteAddress(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://gateway.example/prefix"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/v1/images/generations", nil)
	common.SetContextKey(context, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(context, constant.ContextKeyChannelSetting, dto.ChannelSettings{ProxyImageURLs: true})
	context.Set(common.RequestIdKey, "request-123")

	body, err := common.Marshal(map[string]any{
		"data": []any{
			map[string]any{"url": "https://upstream.example/one.png"},
			map[string]any{"url": "https://upstream.example/two.png", "nested": map[string]any{"file_uri": "https://upstream.example/three.png"}},
			map[string]any{"image_url": map[string]any{"url": "https://upstream.example/four.png"}},
		},
	})
	require.NoError(t, err)

	rewritten := RewriteGeneratedImageURLs(context, body)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(rewritten, &payload))
	data := payload["data"].([]any)

	urls := []string{
		data[0].(map[string]any)["url"].(string),
		data[1].(map[string]any)["url"].(string),
		data[1].(map[string]any)["nested"].(map[string]any)["file_uri"].(string),
		data[2].(map[string]any)["image_url"].(map[string]any)["url"].(string),
	}
	upstreams := []string{
		"https://upstream.example/one.png",
		"https://upstream.example/two.png",
		"https://upstream.example/three.png",
		"https://upstream.example/four.png",
	}

	seen := map[string]bool{}
	for index, raw := range urls {
		require.Contains(t, raw, "https://gateway.example/prefix/v1/artifacts/")
		id := strings.TrimSuffix(strings.TrimPrefix(raw, "https://gateway.example/prefix/v1/artifacts/"), ".png")
		assert.Len(t, id, 22, "the id is a 22-character random token")
		assert.NotContains(t, id, "request-123", "the id must not derive from the caller-visible request id")
		assert.False(t, seen[id], "each URL gets its own id")
		seen[id] = true

		imageURLProxyStore.Lock()
		entry, ok := imageURLProxyStore.entries[id]
		imageURLProxyStore.Unlock()
		require.True(t, ok, "the id resolves to an entry")
		assert.Equal(t, upstreams[index], entry.URL)
	}
}

func TestRewriteGeneratedImageURLsSkipsUnsupportedChannels(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/v1/images/generations", nil)
	common.SetContextKey(context, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
	common.SetContextKey(context, constant.ContextKeyChannelSetting, dto.ChannelSettings{ProxyImageURLs: true})
	context.Set(common.RequestIdKey, "request-123")

	body := []byte(`{"data":[{"url":"https://upstream.example/image.png"}]}`)
	assert.Equal(t, body, RewriteGeneratedImageURLs(context, body))
}
