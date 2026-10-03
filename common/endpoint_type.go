package common

import "github.com/QuantumNous/new-api/constant"

// GetEndpointTypesByChannelType 获取渠道最优先端点类型（所有的渠道都支持 OpenAI 端点）
func GetEndpointTypesByChannelType(channelType int, modelName string) []constant.EndpointType {
	var endpointTypes []constant.EndpointType
	switch channelType {
	case constant.ChannelTypeJina:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeJinaRerank}
	//case constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeMidjourney}
	//case constant.ChannelTypeSunoAPI:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeSuno}
	//case constant.ChannelTypeKling:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeKling}
	//case constant.ChannelTypeJimeng:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeJimeng}
	case constant.ChannelTypeAws:
		fallthrough
	case constant.ChannelTypeAnthropic:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeAnthropic, constant.EndpointTypeOpenAI}
	case constant.ChannelTypeVertexAi:
		fallthrough
	case constant.ChannelTypeGemini:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeGemini, constant.EndpointTypeOpenAI}
	case constant.ChannelTypeOpenRouter: // OpenRouter 只支持 OpenAI 端点
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
	case constant.ChannelTypeXai:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI, constant.EndpointTypeOpenAIResponse}
	case constant.ChannelTypeVLLM, constant.ChannelTypeSGLang:
		endpointTypes = GetAdvancedCustomPreset(channelType).SupportedEndpointTypesForModel(modelName)
	case constant.ChannelTypeSora:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAIVideo}
	case constant.ChannelTypeMeshy2API:
		endpointTypes = []constant.EndpointType{constant.EndpointType3D}
	case constant.ChannelTypeAgnes:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAIVideo}
	case constant.ChannelTypeSub2API, constant.ChannelTypeNewAPI:
		endpointTypes = []constant.EndpointType{
			constant.EndpointTypeOpenAI,
			constant.EndpointTypeOpenAIResponse,
			constant.EndpointTypeOpenAIResponseCompact,
			constant.EndpointTypeAnthropic,
			constant.EndpointTypeGemini,
			constant.EndpointTypeOpenAIAlphaSearch,
		}
	case constant.ChannelTypeCodex:
		endpointTypes = []constant.EndpointType{
			constant.EndpointTypeOpenAIResponse,
			constant.EndpointTypeOpenAIResponseCompact,
			constant.EndpointTypeOpenAIAlphaSearch,
		}
	case constant.ChannelTypeTypeSafe:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeSystemOne}
	case constant.ChannelTypeOpenCode:
		// An OpenCode pool fronts OpenAI and Anthropic traffic as well as the
		// Jev decision endpoint, so all three route shapes are accepted.
		endpointTypes = []constant.EndpointType{
			constant.EndpointTypeOpenAI,
			constant.EndpointTypeAnthropic,
			constant.EndpointTypeSystemOne,
		}
	default:
		if IsOpenAIResponseOnlyModel(modelName) {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAIResponse}
		} else {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
		}
	}
	return WithModelCapabilityEndpoints(endpointTypes, modelName)
}

// WithModelCapabilityEndpoints prepends the endpoint a model's own capability
// implies, ahead of whatever the channel serves by default.
//
// Callers that derive the endpoint list from channel configuration alone must
// apply this too. The Advanced Custom pricing path did not, so its image models
// were advertised as chat endpoints, and the playground offered them — and chat
// models — in the wrong tabs.
func WithModelCapabilityEndpoints(endpointTypes []constant.EndpointType, modelName string) []constant.EndpointType {
	implied := make([]constant.EndpointType, 0, 3)
	if IsImageGenerationModel(modelName) {
		implied = append(implied, constant.EndpointTypeImageGeneration)
	}
	if IsEmbeddingModel(modelName) {
		implied = append(implied, constant.EndpointTypeEmbeddings)
	}
	if IsRerankModel(modelName) {
		implied = append(implied, constant.EndpointTypeJinaRerank)
	}
	if len(implied) == 0 {
		return endpointTypes
	}
	// A channel route may already declare the same endpoint; keep the first
	// occurrence so the list stays a set.
	seen := make(map[constant.EndpointType]struct{}, len(implied)+len(endpointTypes))
	merged := make([]constant.EndpointType, 0, len(implied)+len(endpointTypes))
	for _, endpointType := range append(implied, endpointTypes...) {
		if _, duplicate := seen[endpointType]; duplicate {
			continue
		}
		seen[endpointType] = struct{}{}
		merged = append(merged, endpointType)
	}
	return merged
}
