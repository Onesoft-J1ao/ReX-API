package middleware

import (
	"bytes"

	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// contentReviewResponseWriter tees the relay response body into a bounded
// buffer so content review can extract the assistant reply after the handler
// returns. Streaming responses are captured too: the SSE frames the gateway
// writes are what get buffered, and the extractor parses them back out.
type contentReviewResponseWriter struct {
	gin.ResponseWriter
	body    *bytes.Buffer
	maxSize int
}

func (w *contentReviewResponseWriter) Write(b []byte) (int, error) {
	if w.body.Len() < w.maxSize {
		remain := w.maxSize - w.body.Len()
		if remain >= len(b) {
			w.body.Write(b)
		} else {
			w.body.Write(b[:remain])
		}
	}
	return w.ResponseWriter.Write(b)
}

func (w *contentReviewResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// ContentReviewMaxResponseBytes bounds one captured reply. Review sampling does
// not need the whole stream; 256 KiB covers any reply worth reading.
const ContentReviewMaxResponseBytes = 256 << 10

// ContentReviewCapture tees relay response bodies when the review switch is on.
// The buffered bytes are read by controller.Relay's deferred capture; when the
// switch is off this is a plain pass-through with no writer wrapper, so the
// relay path costs nothing.
func ContentReviewCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !operation_setting.ContentCaptureEnabled() {
			c.Next()
			return
		}
		writer := &contentReviewResponseWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBuffer(nil),
			maxSize:        ContentReviewMaxResponseBytes,
		}
		c.Writer = writer
		c.Next()
		// The consumer (controller.Relay) reads the tee'd bytes after the
		// handler chain returns.
		c.Set("content_review_captured_body", writer.body.Bytes())
	}
}
