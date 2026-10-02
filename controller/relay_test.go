package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/assert"
)

func TestChannelSupportsRelayMode(t *testing.T) {
	cases := []struct {
		name        string
		channelType int
		relayMode   int
		want        bool
	}{
		{
			name:        "TypeSafe serves the decision endpoint",
			channelType: constant.ChannelTypeTypeSafe,
			relayMode:   relayconstant.RelayModeSystemOne,
			want:        true,
		},
		{
			name:        "TypeSafe does not serve ordinary chat",
			channelType: constant.ChannelTypeTypeSafe,
			relayMode:   relayconstant.RelayModeChatCompletions,
			want:        false,
		},
		{
			name:        "OpenCode serves the decision endpoint",
			channelType: constant.ChannelTypeOpenCode,
			relayMode:   relayconstant.RelayModeSystemOne,
			want:        true,
		},
		{
			name:        "OpenCode still serves ordinary chat",
			channelType: constant.ChannelTypeOpenCode,
			relayMode:   relayconstant.RelayModeChatCompletions,
			want:        true,
		},
		{
			name:        "an OpenAI channel cannot serve the decision endpoint",
			channelType: constant.ChannelTypeOpenAI,
			relayMode:   relayconstant.RelayModeSystemOne,
			want:        false,
		},
		{
			name:        "an OpenAI channel still serves ordinary chat",
			channelType: constant.ChannelTypeOpenAI,
			relayMode:   relayconstant.RelayModeChatCompletions,
			want:        true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, channelSupportsRelayMode(tc.channelType, tc.relayMode))
		})
	}
}
