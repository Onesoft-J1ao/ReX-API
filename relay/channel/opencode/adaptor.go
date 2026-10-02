package opencode

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// An OpenCode pool serves the usual OpenAI and Anthropic endpoints plus Jev's
// structured-decision endpoint at the root path. Embedding the OpenAI adaptor
// keeps every chat/messages behaviour identical; only the System One request
// and response paths are added here.
type Adaptor struct {
	openai.Adaptor
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == relayconstant.RelayModeSystemOne {
		if info.RelayFormat != types.RelayFormatSystemOne {
			return "", fmt.Errorf("OpenCode System One requires the systemone relay format")
		}
		return info.ChannelBaseUrl + "/v1/systemone", nil
	}
	return a.Adaptor.GetRequestURL(info)
}

func (a *Adaptor) ConvertSystemOneRequest(_ *gin.Context, info *relaycommon.RelayInfo, request *dto.SystemOneRequest) (*dto.SystemOneRequest, error) {
	if info.RelayFormat != types.RelayFormatSystemOne || info.RelayMode != relayconstant.RelayModeSystemOne {
		return nil, errors.New("OpenCode System One conversion requires a systemone request")
	}
	converted := *request
	converted.Model = info.UpstreamModelName
	return &converted, nil
}

// DoResponse hands System One replies to the shared handler; every other relay
// mode keeps the embedded OpenAI behaviour.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == relayconstant.RelayModeSystemOne {
		return channel.HandleSystemOneResponse(c, resp)
	}
	return a.Adaptor.DoResponse(c, resp, info)
}

func (a *Adaptor) GetChannelName() string { return ChannelName }
