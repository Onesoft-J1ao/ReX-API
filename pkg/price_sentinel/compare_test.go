package pricesentinel

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func fptr(v float64) *float64 { return &v }

// A model synced to upstream * (1 + margin) is healthy and must not be reported.
// Comparing against the raw upstream number flagged every synced price at
// -16.67% (1/1.2 - 1), and applying the report wrote the margin back off — so
// each run undid the previous one.
func TestCompareModelAcceptsAMarkedUpPrice(t *testing.T) {
	ours := localPrice{values: map[string]*float64{
		"model_ratio":      fptr(0.15),
		"completion_ratio": fptr(3),
	}}
	upstream := upstreamPrice{ModelRatio: 0.125, CompletionRatio: 2.5}

	changes := compareModel("grok-4.3", ours, upstream, 5, 0.2)

	assert.Empty(t, changes, "a correctly marked-up price is not a difference")
}

// Drift away from the marked-up expectation is still reported, so the check
// keeps its purpose.
func TestCompareModelReportsDriftFromTheExpectedPrice(t *testing.T) {
	ours := localPrice{values: map[string]*float64{"model_ratio": fptr(0.30)}}
	upstream := upstreamPrice{ModelRatio: 0.125}

	changes := compareModel("grok-4.3", ours, upstream, 5, 0.2)

	// Percent is relative to our own price: the expected 0.15 sits 50% below
	// our 0.30, so the signed value is negative — we are charging more than the
	// marked-up upstream.
	if assert.Len(t, changes, 1) {
		assert.Equal(t, "model_ratio", changes[0].Field)
		assert.InDelta(t, -50, changes[0].Percent, 0.01)
	}
}

// A dimension neither side charges for is silent, even when one side stores it
// as an explicit zero.
func TestCompareModelIgnoresZeroAgainstAbsentBothWays(t *testing.T) {
	ours := localPrice{values: map[string]*float64{
		"model_ratio": fptr(1),
		"cache_ratio": fptr(0),
	}}
	upstream := upstreamPrice{ModelRatio: 1}

	changes := compareModel("m", ours, upstream, 5, 0)

	assert.Empty(t, changes, "0 here against absent upstream is not a difference")
}

// A dimension only one side charges for is a real difference and stays reported.
func TestCompareModelReportsAOneSidedDimension(t *testing.T) {
	ours := localPrice{values: map[string]*float64{
		"model_ratio": fptr(1.2),
		"cache_ratio": fptr(0.08),
	}}
	upstream := upstreamPrice{ModelRatio: 1}

	changes := compareModel("m", ours, upstream, 5, 0.2)

	if assert.Len(t, changes, 1) {
		assert.Equal(t, "cache_ratio", changes[0].Field)
	}
}

func TestPercentChangeHasNoMeaningForAZeroSide(t *testing.T) {
	assert.Equal(t, 0.0, percentChange(0, 5))
	assert.Equal(t, 0.0, percentChange(5, 0))
	assert.Equal(t, 0.0, percentChange(0, 0))
	assert.InDelta(t, 100, percentChange(1, 2), 0.001)
}

// WithMargin is what the apply path writes, so it has to agree with the
// expectation compareModel uses; a mismatch would make every apply re-report.
func TestWithMarginMatchesTheComparisonExpectation(t *testing.T) {
	upstream := 0.125
	written := WithMargin(upstream, 0.2)
	expected := upstream * (1 + 0.2)

	assert.InDelta(t, expected, written, 1e-6)

	// And the written value compares clean against the same upstream record.
	ours := localPrice{values: map[string]*float64{"model_ratio": &written}}
	changes := compareModel("m", ours, upstreamPrice{ModelRatio: upstream}, 5, 0.2)
	assert.Empty(t, changes)
}

var _ = model.PriceChange{}
