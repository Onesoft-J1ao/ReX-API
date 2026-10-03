package common_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestWanEndpointsDistinguishImagesFromVideos(t *testing.T) {
	for _, name := range []string{
		"wan2.7-image-pro", "wan2.7-image", "wan2.6-image", "wan2.6-t2i",
		"wan2.5-t2i-preview", "wan2.2-t2i-flash", "wan2.2-t2i-plus",
		"wanx2.1-t2i-turbo", "wanx2.1-t2i-plus", "wanx2.0-t2i-turbo",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, common.GetEndpointTypesByChannelType(constant.ChannelTypeAli, name), constant.EndpointTypeImageGeneration)
		})
	}
	for _, name := range []string{
		"wanx2.1-t2v-plus", "wanx2.1-t2v-turbo", "wanx2.1-i2v-plus", "wanx2.1-i2v-turbo",
	} {
		t.Run(name, func(t *testing.T) {
			assert.NotContains(t, common.GetEndpointTypesByChannelType(constant.ChannelTypeAli, name), constant.EndpointTypeImageGeneration)
		})
	}
}

func TestEmbeddingAndRerankEndpointsComeFirst(t *testing.T) {
	// A chat endpoint declared ahead of the endpoint a model actually speaks
	// makes the channel test probe the wrong path and report the model red.
	for _, name := range []string{
		"nvidia/embed-qa-4", "nvidia/nemotron-3-embed-1b", "snowflake/arctic-embed-l",
		"text-embedding-3-large", "BAAI/bge-m3", "m3e-base",
	} {
		t.Run(name, func(t *testing.T) {
			endpoints := common.GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, name)
			assert.NotEmpty(t, endpoints)
			assert.Equal(t, constant.EndpointTypeEmbeddings, endpoints[0])
		})
	}

	for _, name := range []string{"bge-reranker-v2-m3", "jina-reranker-v2", "Qwen3-Reranker-8B"} {
		t.Run(name, func(t *testing.T) {
			endpoints := common.GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, name)
			assert.NotEmpty(t, endpoints)
			assert.Equal(t, constant.EndpointTypeJinaRerank, endpoints[0])
		})
	}

	// "bge-reranker" carries the bge embedding marker, so the rerank check must
	// win; otherwise it is probed as an embedding model.
	assert.False(t, common.IsEmbeddingModel("bge-reranker-v2-m3"))
	assert.True(t, common.IsRerankModel("bge-reranker-v2-m3"))
	assert.True(t, common.IsEmbeddingModel("BAAI/bge-m3"))
}
