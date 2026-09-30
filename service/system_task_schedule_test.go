package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

type intervalOnlyHandler struct{ interval time.Duration }

func (intervalOnlyHandler) Type() string                                   { return "test.interval" }
func (intervalOnlyHandler) Run(context.Context, *model.SystemTask, string) {}
func (intervalOnlyHandler) Enabled() bool                                  { return true }
func (h intervalOnlyHandler) Interval() time.Duration                      { return h.interval }
func (intervalOnlyHandler) NewPayload() any                                { return nil }

type anchoredHandler struct {
	intervalOnlyHandler
	next time.Time
}

func (h anchoredHandler) NextRunAfter(time.Time) time.Time { return h.next }

// A handler that only reports an interval keeps the plain elapsed-time rule, so
// adding the anchor hook left every existing schedule untouched.
func TestScheduledTaskIsDueUsesTheIntervalWhenUnanchored(t *testing.T) {
	handler := intervalOnlyHandler{interval: time.Hour}
	lastRun := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	assert.False(t, scheduledTaskIsDue(handler, lastRun, lastRun.Add(59*time.Minute)))
	assert.True(t, scheduledTaskIsDue(handler, lastRun, lastRun.Add(time.Hour)))
}

// A handler that anchors its cadence decides from the anchor, and its interval
// no longer fires it: here the anchor is later than the interval would have.
func TestScheduledTaskIsDuePrefersTheAnchor(t *testing.T) {
	lastRun := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	anchor := lastRun.Add(30 * time.Minute)
	handler := anchoredHandler{
		intervalOnlyHandler: intervalOnlyHandler{interval: time.Minute},
		next:                anchor,
	}

	assert.False(t, scheduledTaskIsDue(handler, lastRun, lastRun.Add(29*time.Minute)),
		"the interval alone must not fire an anchored handler")
	assert.True(t, scheduledTaskIsDue(handler, lastRun, anchor))
}
