package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const logQueueTestTimeout = 10 * time.Second

func logQueueTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), logQueueTestTimeout)
	t.Cleanup(cancel)
	return ctx
}

// startEphemeralLogQueue installs a queue for one test and guarantees it is
// drained and stopped afterwards, so a test can never leak a writer goroutine
// into the next one.
func startEphemeralLogQueue(t *testing.T, size int) {
	t.Helper()
	ctx := logQueueTestContext(t)
	require.NoError(t, StopLogQueue(ctx))
	previousSize := common.LogQueueSize
	common.LogQueueSize = size
	StartLogQueue()
	t.Cleanup(func() {
		common.LogQueueSize = previousSize
		stopCtx, cancel := context.WithTimeout(context.Background(), logQueueTestTimeout)
		defer cancel()
		require.NoError(t, StopLogQueue(stopCtx))
	})
}

func newQueuedLog(userId int, content string) *Log {
	return &Log{
		UserId:    userId,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeConsume,
		Content:   content,
	}
}

func logsForUser(t *testing.T, userId int) []*Log {
	t.Helper()
	var logs []*Log
	require.NoError(t, LOG_DB.Where("user_id = ?", userId).Order("id asc").Find(&logs).Error)
	return logs
}

func TestCreateLogWithoutQueueWritesSynchronously(t *testing.T) {
	// Unit tests and CLI paths run without a queue; createLog must stay an
	// inline insert there or every existing test that reads a log back would
	// have to know about flushing.
	logQueue.Store(nil)
	userId := 91001
	t.Cleanup(func() { LOG_DB.Where("user_id = ?", userId).Delete(&Log{}) })

	require.NoError(t, createLog(newQueuedLog(userId, "sync")))
	assert.Len(t, logsForUser(t, userId), 1)
}

func TestLogQueuePreservesOrderAndWritesEveryRecord(t *testing.T) {
	startEphemeralLogQueue(t, 64)
	userId := 91002
	t.Cleanup(func() { LOG_DB.Where("user_id = ?", userId).Delete(&Log{}) })

	const total = 200
	for i := range total {
		require.NoError(t, createLog(newQueuedLog(userId, fmt.Sprintf("ordered-%04d", i))))
	}
	require.NoError(t, FlushLogQueue(logQueueTestContext(t)))

	logs := logsForUser(t, userId)
	require.Len(t, logs, total)
	for i, log := range logs {
		assert.Equal(t, fmt.Sprintf("ordered-%04d", i), log.Content, "row %d was written out of order", i)
	}
}

// A queue smaller than the burst forces producers to wait on a full channel.
// The rows must still all arrive: the queue is a backpressure buffer, never a
// lossy one, because dropping consume/task logs breaks the billing audit.
func TestLogQueueBackpressureDoesNotDropRecords(t *testing.T) {
	startEphemeralLogQueue(t, 1)
	userId := 91003
	t.Cleanup(func() { LOG_DB.Where("user_id = ?", userId).Delete(&Log{}) })

	const total = 50
	for range total {
		require.NoError(t, createLog(newQueuedLog(userId, "pressure")))
	}
	require.NoError(t, FlushLogQueue(logQueueTestContext(t)))
	assert.Len(t, logsForUser(t, userId), total)
}

func TestStopLogQueueDrainsPendingRecords(t *testing.T) {
	startEphemeralLogQueue(t, 256)
	userId := 91004
	t.Cleanup(func() { LOG_DB.Where("user_id = ?", userId).Delete(&Log{}) })

	const total = 100
	for range total {
		require.NoError(t, createLog(newQueuedLog(userId, "drain")))
	}
	ctx := logQueueTestContext(t)
	require.NoError(t, StopLogQueue(ctx))

	assert.Len(t, logsForUser(t, userId), total)
}

// After the queue stops, handlers that are still finishing must not lose their
// log: createLog has to fall back to the synchronous insert.
func TestCreateLogAfterQueueStopWritesSynchronously(t *testing.T) {
	startEphemeralLogQueue(t, 64)
	userId := 91005
	t.Cleanup(func() { LOG_DB.Where("user_id = ?", userId).Delete(&Log{}) })

	ctx := logQueueTestContext(t)
	require.NoError(t, StopLogQueue(ctx))

	require.NoError(t, createLog(newQueuedLog(userId, "late")))
	assert.Len(t, logsForUser(t, userId), 1)
}
