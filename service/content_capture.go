package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// Content review capture. Bodies are only stored when the operator turned the
// switch on; every write funnels through RecordContentForRequest, which is a
// no-op (and cheap) otherwise.

const maxCaptureMessageLen = 32 << 10 // per-message cut before joining

// ExtractUserTextFromOpenAIRequest renders the user-visible text of an OpenAI
// style request: one line per message as "role: content", skipping empty and
// tool payloads. Media parts contribute a placeholder so a review reader can
// tell an image was there.
func ExtractUserTextFromOpenAIRequest(request *dto.GeneralOpenAIRequest) string {
	if request == nil {
		return ""
	}
	var b strings.Builder
	if len(request.Messages) > 0 {
		for _, msg := range request.Messages {
			text := strings.TrimSpace(msg.StringContent())
			if text == "" {
				text = mediaPlaceholder(msg.Content)
			}
			if text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "[%s] %s", msg.Role, clip(text, maxCaptureMessageLen))
		}
		return b.String()
	}
	// completions / responses style: prompt or input.
	if text := anyText(request.Prompt); text != "" {
		return clip(text, model.MaxStoredContentBytes)
	}
	if text := anyText(request.Input); text != "" {
		return clip(text, model.MaxStoredContentBytes)
	}
	return ""
}

// ExtractUserTextFromClaudeRequest renders a Claude Messages request the same
// way. System goes first as its own line.
func ExtractUserTextFromClaudeRequest(request *dto.ClaudeRequest) string {
	if request == nil {
		return ""
	}
	var b strings.Builder
	if sys := claudeSystemText(request.System); sys != "" {
		fmt.Fprintf(&b, "[system] %s\n", clip(sys, maxCaptureMessageLen))
	}
	for _, msg := range request.Messages {
		text := strings.TrimSpace(msg.GetStringContent())
		if text == "" {
			text = claudeMediaPlaceholder(msg.Content)
		}
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "[%s] %s", msg.Role, clip(text, maxCaptureMessageLen))
	}
	return b.String()
}

func claudeSystemText(system any) string {
	switch v := system.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok && text != "" {
					if b.Len() > 0 {
						b.WriteString("\n")
					}
					b.WriteString(text)
				}
			}
		}
		return b.String()
	}
	return ""
}

func mediaPlaceholder(content any) string {
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	seen := make([]string, 0, len(parts))
	for _, part := range parts {
		m, ok := part.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "image_url", "input_image":
			seen = append(seen, "[image]")
		case "input_audio", "audio":
			seen = append(seen, "[audio]")
		case "file":
			seen = append(seen, "[file]")
		}
	}
	return strings.Join(seen, "")
}

func claudeMediaPlaceholder(content any) string {
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	seen := make([]string, 0, len(parts))
	for _, part := range parts {
		m, ok := part.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "image":
			seen = append(seen, "[image]")
		case "document":
			seen = append(seen, "[file]")
		}
	}
	return strings.Join(seen, "")
}

func anyText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		// Responses API input: an array of {role, content:[{type,text}]} objects.
		var b strings.Builder
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			content := m["content"]
			var text string
			switch c := content.(type) {
			case string:
				text = c
			case []any:
				var pieces []string
				for _, p := range c {
					if pm, ok := p.(map[string]any); ok {
						if t, ok := pm["text"].(string); ok && t != "" {
							pieces = append(pieces, t)
						}
					}
				}
				text = strings.Join(pieces, " ")
			}
			if text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "[%s] %s", role, clip(text, maxCaptureMessageLen))
		}
		return b.String()
	}
	return ""
}

func clip(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}

// ExtractResponseTextFromUpstreamBody pulls the assistant-visible reply out of
// a non-stream upstream JSON body. Works for OpenAI chat, Claude Messages and
// Gemini-style candidates; the first shape that parses wins.
func ExtractResponseTextFromUpstreamBody(body []byte) string {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 || body[0] != '{' {
		return ""
	}
	// OpenAI chat: choices[0].message.content
	var openai struct {
		Choices []struct {
			Message struct {
				Content          any    `json:"content"`
				ReasoningContent string `json:"reasoning_content,omitempty"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := common.Unmarshal(body, &openai); err == nil && len(openai.Choices) > 0 {
		choice := openai.Choices[0]
		if text := anyText(choice.Message.Content); text != "" {
			return text
		}
		if choice.Text != "" {
			return choice.Text
		}
	}
	// Claude Messages: content[] of {type:"text",text}
	var claude struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := common.Unmarshal(body, &claude); err == nil && len(claude.Content) > 0 {
		var b strings.Builder
		for _, block := range claude.Content {
			if block.Type == "text" && block.Text != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(block.Text)
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	// Gemini: candidates[0].content.parts[].text
	var gemini struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := common.Unmarshal(body, &gemini); err == nil && len(gemini.Candidates) > 0 {
		var b strings.Builder
		for _, part := range gemini.Candidates[0].Content.Parts {
			if part.Text != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(part.Text)
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	return ""
}

// ExtractResponseTextFromSSE pulls the assistant reply out of an upstream SSE
// stream by re-parsing each data frame. Cheap enough for review sampling: the
// body is already in memory when we buffered it, and for pass-through streams
// the frames are replayed from the tee.
func ExtractResponseTextFromSSE(frames []string) string {
	var b strings.Builder
	for _, frame := range frames {
		frame = strings.TrimSpace(frame)
		if !strings.HasPrefix(frame, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(frame, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		// OpenAI chunk
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          any    `json:"content"`
					ReasoningContent string `json:"reasoning_content,omitempty"`
				} `json:"delta"`
				Text string `json:"text"`
			} `json:"choices"`
		}
		if err := common.Unmarshal([]byte(payload), &chunk); err == nil && len(chunk.Choices) > 0 {
			if text := anyText(chunk.Choices[0].Delta.Content); text != "" {
				b.WriteString(text)
			} else if chunk.Choices[0].Text != "" {
				b.WriteString(chunk.Choices[0].Text)
			}
			continue
		}
		// Claude chunk: content_block_delta
		var claudeChunk struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := common.Unmarshal([]byte(payload), &claudeChunk); err == nil &&
			claudeChunk.Type == "content_block_delta" && claudeChunk.Delta.Text != "" {
			b.WriteString(claudeChunk.Delta.Text)
			continue
		}
		// Gemini stream: candidates[].content.parts[].text
		var geminiChunk struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := common.Unmarshal([]byte(payload), &geminiChunk); err == nil &&
			len(geminiChunk.Candidates) > 0 {
			for _, part := range geminiChunk.Candidates[0].Content.Parts {
				b.WriteString(part.Text)
			}
		}
	}
	return b.String()
}

// StoreRequestContent stores the prompt half of an exchange. Called from the
// relay entry once the parsed request is available. A JSON parse here is fine —
// the request structs are already parsed by the distributor.
func StoreRequestContent(c any, requestId string, userId int, modelName, tokenName, requestText string) {
	if !operation_setting.ContentCaptureEnabled() || requestId == "" || requestText == "" {
		return
	}
	go model.UpsertLogContent(requestId, int64(userId), modelName, tokenName, requestText, "")
}

// StoreResponseContent merges the assistant reply into the row created above.
// The goroutine ordering is request-first: UpsertLogContent uses FirstOrCreate
// with Assign, so whichever side lands first creates the row and the other
// fills its half.
func StoreResponseContent(requestId string, userId int, modelName, tokenName, responseText string) {
	if !operation_setting.ContentCaptureEnabled() || requestId == "" || responseText == "" {
		return
	}
	go model.UpsertLogContent(requestId, int64(userId), modelName, tokenName, "", responseText)
}
