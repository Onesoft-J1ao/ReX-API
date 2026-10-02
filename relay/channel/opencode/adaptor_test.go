package opencode

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLRoutesSystemOneToTheRootPath(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "http://127.0.0.1:10000",
			UpstreamModelName: "opencode/jev-1.13-free",
		},
		RelayFormat: types.RelayFormatSystemOne,
		RelayMode:   relayconstant.RelayModeSystemOne,
	}

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:10000/v1/systemone", url)

	// A systemone relay mode arriving in another format is a routing bug upstream.
	info.RelayFormat = types.RelayFormatOpenAI
	_, err = adaptor.GetRequestURL(info)
	assert.Error(t, err)
}

func TestConvertSystemOneRequestUsesTheUpstreamModelName(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "jev-1.13-free"},
		RelayFormat: types.RelayFormatSystemOne,
		RelayMode:   relayconstant.RelayModeSystemOne,
	}
	request := &dto.SystemOneRequest{
		Model: "opencode/jev-1.13-free",
		State: []byte(`"payments failed"`),
	}

	converted, err := adaptor.ConvertSystemOneRequest(nil, info, request)
	require.NoError(t, err)
	assert.Equal(t, "jev-1.13-free", converted.Model)
	// The original request must not be mutated.
	assert.Equal(t, "opencode/jev-1.13-free", request.Model)

	info.RelayMode = relayconstant.RelayModeChatCompletions
	_, err = adaptor.ConvertSystemOneRequest(nil, info, request)
	assert.Error(t, err)
}

func TestOpenCodeRegistrationUsesStableChannelType(t *testing.T) {
	assert.Equal(t, 68, constant.ChannelTypeOpenCode)
	assert.Less(t, constant.ChannelTypeOpenCode, constant.ChannelTypeDummy)
}
