package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
	"time"

	rootcommon "github.com/QuantumNous/new-api/common"
)

// firstByteDeadline bounds the wait for the first byte of an upstream response
// on streaming relay requests.
//
// http.Client.Timeout cannot be used for this: it stays armed for the whole
// response, so any value short enough to beat Cloudflare's ~100s 524 would also
// truncate every healthy long-lived SSE stream. A context deadline has the same
// problem. This deadline is instead disarmed by the transport itself: the timer
// fires only if the upstream has not produced the first byte of its response
// headers within the bound, and the instant that byte arrives the timer stops.
// Once headers are in, the remaining lifetime of the connection is stream
// duration and is left completely unbounded.
//
// Only streaming requests are wrapped. A non-streaming response's "first byte"
// is the completed generation, so a bound there would cut legitimate long
// calls; see common.RelayStreamFirstByteTimeout.
type firstByteDeadline struct {
	timedOut atomic.Bool
	timer    *time.Timer
	cancel   context.CancelFunc
}

// newFirstByteDeadline returns a context carrying the deadline trace. The
// returned handle must be released once the response body is closed.
func newFirstByteDeadline(ctx context.Context, bound time.Duration) (context.Context, *firstByteDeadline) {
	deadlineCtx, cancel := context.WithCancel(ctx)
	deadline := &firstByteDeadline{cancel: cancel}
	deadline.timer = time.AfterFunc(bound, func() {
		deadline.timedOut.Store(true)
		cancel()
	})
	trace := &httptrace.ClientTrace{
		// Headers arrived: cancel the deadline so the stream body can run as long
		// as the upstream needs.
		GotFirstResponseByte: func() { deadline.timer.Stop() },
	}
	return httptrace.WithClientTrace(deadlineCtx, trace), deadline
}

// release tears down a deadline whose response body has been closed. It is safe
// on a nil handle and safe to call more than once.
func (d *firstByteDeadline) release() {
	if d == nil {
		return
	}
	d.timer.Stop()
	d.cancel()
}

// firstByteTimeoutBody releases the deadline when the response body is closed,
// so the derived context cannot outlive the request it bounds.
type firstByteTimeoutBody struct {
	rc      io.ReadCloser
	release func()
}

func (b *firstByteTimeoutBody) Read(p []byte) (int, error) {
	return b.rc.Read(p)
}

func (b *firstByteTimeoutBody) Close() error {
	err := b.rc.Close()
	b.release()
	return err
}

// wrapStreamFirstByteDeadline applies common.RelayStreamFirstByteTimeout to a
// streaming request. It returns the request (unwrapped when the bound is
// disabled) and the deadline handle whose release must be wired to the response
// body; the handle is nil when no bound applies.
func wrapStreamFirstByteDeadline(req *http.Request, isStream bool) (*http.Request, *firstByteDeadline) {
	if !isStream || req == nil {
		return req, nil
	}
	seconds := rootcommon.RelayStreamFirstByteTimeout
	if seconds <= 0 {
		return req, nil
	}
	traceCtx, deadline := newFirstByteDeadline(req.Context(), time.Duration(seconds)*time.Second)
	return req.WithContext(traceCtx), deadline
}
