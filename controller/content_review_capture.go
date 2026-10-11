package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// ContentReviewCapturedBodyKey is set by middleware.ContentReviewCapture; it is
// declared here (the consumer) so the middleware has no controller dependency.
const ContentReviewCapturedBodyKey = "content_review_captured_body"

// captureReviewContent stores the request and reply bodies of one relay call
// for human review. Called from Relay's success path, after the handler has
// written the response: the request struct is already parsed, and the reply is
// in the tee'd writer the ContentReviewCapture middleware installed (only when
// the review switch is on).
//
// The user-facing usage-log row carries nothing but ids; the bodies live in
// log_contents, behind admin-only endpoints.
func captureReviewContent(c *gin.Context, relayFormat types.RelayFormat, request dto.Request, requestId string, userId int, modelName, tokenName string) {
	if !operation_setting.ContentCaptureEnabled() || requestId == "" {
		return
	}

	requestText := extractRequestText(relayFormat, request)
	responseText := extractResponseText(c)

	if requestText == "" && responseText == "" {
		return
	}
	go model.UpsertLogContent(requestId, int64(userId), modelName, tokenName, requestText, responseText)
}

func extractRequestText(relayFormat types.RelayFormat, request dto.Request) string {
	switch req := request.(type) {
	case *dto.GeneralOpenAIRequest:
		return service.ExtractUserTextFromOpenAIRequest(req)
	case *dto.ClaudeRequest:
		return service.ExtractUserTextFromClaudeRequest(req)
	}
	return ""
}

// extractResponseText pulls the assistant reply out of the buffered response
// body. Non-stream JSON bodies parse directly; SSE bodies (the frames the
// gateway streamed to the client) are re-parsed frame by frame.
func extractResponseText(c *gin.Context) string {
	raw, ok := c.Get(ContentReviewCapturedBodyKey)
	if !ok {
		return ""
	}
	body, ok := raw.([]byte)
	if !ok || len(body) == 0 {
		return ""
	}
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return service.ExtractResponseTextFromUpstreamBody([]byte(trimmed))
	}
	// SSE: split frames on the blank line; each frame starts with "data:".
	frames := strings.Split(trimmed, "\n\n")
	return service.ExtractResponseTextFromSSE(frames)
}
