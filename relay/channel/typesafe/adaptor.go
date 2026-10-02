package typesafe

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

type Adaptor struct{}

func (a *Adaptor) Init(*relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayFormat != types.RelayFormatSystemOne || info.RelayMode != relayconstant.RelayModeSystemOne {
		return "", fmt.Errorf("TypeSafe supports only System One requests")
	}
	return info.ChannelBaseUrl + "/zen/v1/systemone", nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertSystemOneRequest(_ *gin.Context, info *relaycommon.RelayInfo, request *dto.SystemOneRequest) (*dto.SystemOneRequest, error) {
	if info.RelayFormat != types.RelayFormatSystemOne || info.RelayMode != relayconstant.RelayModeSystemOne {
		return nil, errors.New("TypeSafe supports only System One requests")
	}
	converted := *request
	converted.Model = info.UpstreamModelName
	return &converted, nil
}

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("not implemented: TypeSafe does not support OpenAI requests")
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("not implemented: TypeSafe does not support rerank requests")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("not implemented: TypeSafe does not support embedding requests")
}

func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented: TypeSafe does not support audio requests")
}

func (a *Adaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, errors.New("not implemented: TypeSafe does not support image requests")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented: TypeSafe does not support Responses requests")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("not implemented: TypeSafe does not support Claude requests")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("not implemented: TypeSafe does not support Gemini requests")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	return channel.HandleSystemOneResponse(c, resp)
}

func (a *Adaptor) GetModelList() []string { return ModelList }

func (a *Adaptor) GetChannelName() string { return ChannelName }
