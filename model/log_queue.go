package model

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Async log writer.
//
// createLog used to insert into LOG_DB inline on the request goroutine, so the
// completion path of every relay request paid a full database round-trip before
// the handler could return. When LOG_DB is a separately configured log database
// (LOG_SQL_DSN, including ClickHouse) that is a network round-trip; when it is
// the default SQLite file it is worse in practice, because the DSN uses
// `_txlock=immediate` with a 30s busy timeout, so the insert competes with the
// quota batch updater and every other writer for the single SQLite write lock
// and may wait seconds for it. Behind Cloudflare every one of those seconds is
// spent inside the origin's ~100s response budget.
//
// The queue moves that write off the request goroutine. It preserves the
// previous semantics that matter for billing audit:
//
//   - ordering: one writer drains a FIFO channel, so rows are inserted in the
//     order the handlers produced them (the same "order the goroutines happened
//     to run" ordering as before, but with a single serialization point);
//   - completeness: enqueue blocks until the writer accepts the record, so a
//     full queue applies backpressure instead of dropping consume/task logs;
//   - no silent loss: once the queue is stopped (shutdown), createLog falls back
//     to a synchronous insert, so a log produced during the drain window is
//     still persisted.
//
// The behaviour when the buffer is full is therefore "block the producer, and
// report the pressure", not "drop". `blocked` counts the enqueues that had to
// wait on a full channel and `failed` counts rows the database rejected; both
// are surfaced in the backend log, and any rows left unwritten because shutdown
// timed out are reported as lost with an explicit count.
type logWriterQueue struct {
	ch     chan *Log
	stopCh chan struct{}
	done   chan struct{}

	// gate serializes enqueue against stop. A producer holds RLock across the
	// channel send, so once stop holds the write lock no producer can be
	// mid-send and the drain below is guaranteed to see every accepted record.
	gate    sync.RWMutex
	stopped atomic.Bool

	// pending counts records accepted by enqueue but not yet written, so
	// FlushLogQueue can wait for a quiescent queue without racing the writer.
	pending      atomic.Int64
	enqueued     atomic.Uint64
	written      atomic.Uint64
	failed       atomic.Uint64
	blocked      atomic.Uint64
	syncFallback atomic.Uint64
}

var (
	logQueueMu sync.Mutex
	logQueue   atomic.Pointer[logWriterQueue]
)

// StartLogQueue installs the asynchronous log writer. It is called once during
// startup; the queue stays installed after StopLogQueue so late callers keep
// seeing it and take the synchronous fallback path.
func StartLogQueue() {
	logQueueMu.Lock()
	defer logQueueMu.Unlock()
	if q := logQueue.Load(); q != nil && !q.stopped.Load() {
		return
	}
	size := common.LogQueueSize
	if size <= 0 {
		size = defaultLogQueueSize
	}
	q := &logWriterQueue{
		ch:     make(chan *Log, size),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
	logQueue.Store(q)
	go q.run()
	common.SysLog(fmt.Sprintf("async log queue started with capacity %d", size))
}

const defaultLogQueueSize = 4096

// enqueue hands one record to the writer. It blocks while the write keeps up,
// which is the intended backpressure: losing a consume or task log would break
// the billing audit trail.
func (q *logWriterQueue) enqueue(log *Log) error {
	q.gate.RLock()
	defer q.gate.RUnlock()
	if q.stopped.Load() {
		q.syncFallback.Add(1)
		return createLogSync(log)
	}
	if len(q.ch) == cap(q.ch) {
		if n := q.blocked.Add(1); n == 1 || n%100 == 0 {
			common.SysError(fmt.Sprintf(
				"log queue full (capacity %d): request goroutines are waiting on log persistence; blocked=%d failed=%d",
				cap(q.ch), n, q.failed.Load(),
			))
		}
	}
	q.pending.Add(1)
	q.ch <- log
	q.enqueued.Add(1)
	return nil
}

func (q *logWriterQueue) run() {
	for {
		select {
		case log := <-q.ch:
			q.write(log)
			q.pending.Add(-1)
		case <-q.stopCh:
			for {
				select {
				case log := <-q.ch:
					q.write(log)
					q.pending.Add(-1)
				default:
					close(q.done)
					return
				}
			}
		}
	}
}

func (q *logWriterQueue) write(log *Log) {
	if err := createLogSync(log); err != nil {
		q.failed.Add(1)
		common.SysError(fmt.Sprintf("failed to write queued log: %s (failed=%d)", err.Error(), q.failed.Load()))
		return
	}
	q.written.Add(1)
}

// StopLogQueue stops accepting new records, drains everything already queued
// and waits for the writer to exit. Records produced after it returns are
// written synchronously by createLog. If ctx expires first the number of rows
// that could not be flushed is reported as lost.
func StopLogQueue(ctx context.Context) error {
	logQueueMu.Lock()
	q := logQueue.Load()
	logQueueMu.Unlock()
	if q == nil || q.stopped.Load() {
		return nil
	}

	q.gate.Lock()
	q.stopped.Store(true)
	close(q.stopCh)
	q.gate.Unlock()

	select {
	case <-q.done:
		common.SysLog(fmt.Sprintf(
			"async log queue drained: enqueued=%d written=%d failed=%d sync_fallback=%d",
			q.enqueued.Load(), q.written.Load(), q.failed.Load(), q.syncFallback.Load(),
		))
		return nil
	case <-ctx.Done():
		pending := q.pending.Load()
		if pending > 0 {
			common.SysError(fmt.Sprintf(
				"async log queue shutdown timed out with %d unwritten logs; these rows are lost (failed=%d)",
				pending, q.failed.Load(),
			))
		}
		return ctx.Err()
	}
}

// FlushLogQueue blocks until every record accepted so far has been written.
func FlushLogQueue(ctx context.Context) error {
	q := logQueue.Load()
	if q == nil {
		return nil
	}
	for {
		if q.pending.Load() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
}
