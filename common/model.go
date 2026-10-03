package common

import "strings"

var (
	// OpenAIResponseOnlyModels is a list of models that are only available for OpenAI responses.
	OpenAIResponseOnlyModels = []string{
		"o3-pro",
		"o3-deep-research",
		"o4-mini-deep-research",
	}
	ImageGenerationModels = []string{
		// Substring match on "image" covers the "-image" families the gateway
		// serves as image models but never named here: gemini-*-image,
		// agnes-image-*, grok-imagine-image. Every model this instance routes
		// whose name contains "image" is one.
		"image",
		"kolors",
		"dall-e-3",
		"dall-e-2",
		"prefix:dall-e", // Deprecated upstream models; retained for compatible routes.
		// Substring match, so this also covers the "gpt-image-*" family.
		"gpt-image",
		"qwen-image",
		"z-image",
		"wan2.7-image-pro",
		"wan2.7-image",
		"wan2.6-image",
		"wan2.6-t2i",
		"wan2.5-t2i-preview",
		"wan2.2-t2i-flash",
		"wan2.2-t2i-plus",
		"wanx2.1-t2i-turbo",
		"wanx2.1-t2i-plus",
		"wanx2.0-t2i-turbo",
		"prefix:imagen-",
		"flux-",
		"flux.1-",
	}
	OpenAITextModels = []string{
		"gpt-",
		"o1",
		"o3",
		"o4",
		"chatgpt",
	}
)

func IsOpenAIResponseOnlyModel(modelName string) bool {
	for _, m := range OpenAIResponseOnlyModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

func IsImageGenerationModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range ImageGenerationModels {
		if prefix, ok := strings.CutPrefix(m, "prefix:"); ok {
			if strings.HasPrefix(modelName, prefix) {
				return true
			}
			continue
		}
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

// IsEmbeddingModel reports whether a model name denotes an embedding model. The
// rerank check comes first because "bge-reranker" also carries the "bge-"
// embedding marker.
func IsEmbeddingModel(modelName string) bool {
	lower := strings.ToLower(modelName)
	if strings.Contains(lower, "rerank") {
		return false
	}
	return strings.Contains(lower, "embed") ||
		strings.Contains(lower, "bge-") ||
		strings.HasPrefix(lower, "m3e")
}

// IsRerankModel reports whether a model name denotes a rerank model.
func IsRerankModel(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "rerank")
}

func IsOpenAITextModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range OpenAITextModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}
