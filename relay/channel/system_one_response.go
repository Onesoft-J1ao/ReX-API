package channel

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// HandleSystemOneResponse forwards a Jev System One response body to the client
// and returns its usage. The endpoint reports input_tokens/output_tokens only —
// no cache, image or audio breakdown — so those stay zero and the billing path
// treats the request as plain prompt/completion tokens.
func HandleSystemOneResponse(c *gin.Context, resp *http.Response) (any, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("empty System One response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewOpenAIError(readErr, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	var payload map[string]json.RawMessage
	if jsonErr := json.Unmarshal(body, &payload); jsonErr != nil || payload == nil {
		return nil, types.NewOpenAIError(errors.New("invalid System One JSON response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	usageInfo := &dto.Usage{}
	if raw, ok := payload["usage"]; ok {
		var values struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		}
		if json.Unmarshal(raw, &values) == nil {
			usageInfo.InputTokens = values.InputTokens
			usageInfo.OutputTokens = values.OutputTokens
			usageInfo.PromptTokens = values.InputTokens
			usageInfo.CompletionTokens = values.OutputTokens
			usageInfo.TotalTokens = values.InputTokens + values.OutputTokens
			usageInfo.UsageSource = "upstream"
			usageInfo.UsageSemantic = dto.BillingUsageSemanticOpenAI
		}
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	if _, err := c.Writer.Write(body); err != nil {
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry())
	}
	return usageInfo, nil
}
