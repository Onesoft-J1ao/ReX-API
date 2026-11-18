package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestApplyUsagePostProcessingNormalizesStandardCacheFields(t *testing.T) {
	usage := &dto.Usage{
		PromptCacheHitTokens: 31,
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens:     30,
			CacheWriteTokens: 12,
		},
	}

	ApplyUsagePostProcessing(&relaycommon.RelayInfo{ChannelType: constant.ChannelTypeOpenAI}, usage, nil)

	require.Equal(t, 30, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 12, usage.PromptTokensDetails.CacheWriteTokens)
}

func TestApplyUsagePostProcessingExtractsStandardCacheAliases(t *testing.T) {
	usage := &dto.Usage{}
	response := []byte(`{"usage":{"cached_tokens":17,"cache_write_tokens":9}}`)

	ApplyUsagePostProcessing(&relaycommon.RelayInfo{ChannelType: constant.ChannelTypeOpenAI}, usage, response)

	require.Equal(t, 17, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 9, usage.PromptTokensDetails.CacheWriteTokens)
}
