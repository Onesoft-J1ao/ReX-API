package xai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestXAIUsageMergePreservesCacheDetails(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelType: constant.ChannelTypeOpenAI}
	accumulated := &dto.Usage{}
	terminalUsage := &dto.Usage{
		PromptTokens:         5080,
		TotalTokens:          5241,
		PromptCacheHitTokens: 4768,
		PromptTokensDetails:  dto.InputTokenDetails{CacheWriteTokens: 161},
	}

	openai.ApplyUsagePostProcessing(info, terminalUsage, nil)
	dto.MergeUsageNonZero(accumulated, terminalUsage)

	require.Equal(t, 4768, accumulated.PromptTokensDetails.CachedTokens)
	require.Equal(t, 161, accumulated.PromptTokensDetails.CacheWriteTokens)
	require.Equal(t, 5080, accumulated.PromptTokens)
	require.Equal(t, 5241, accumulated.TotalTokens)
}
