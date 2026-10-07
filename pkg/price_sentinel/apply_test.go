package pricesentinel

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](value T) *T {
	return &value
}

// A margin multiplication leaves float noise that would be written straight into
// the price table and then reported as a difference on the next run, so the
// rounding is part of the contract rather than cosmetic.
func TestChangeIsReportedOnlyMatchesTheMailedReport(t *testing.T) {
	changes := []model.PriceChange{
		{ModelName: "grok-4.6", Field: "model_ratio"},
	}

	assert.True(t, ChangeIsReported(changes, " grok-4.6 ", "model_ratio"))
	assert.False(t, ChangeIsReported(changes, "other", "model_ratio"))
	assert.False(t, ChangeIsReported(changes, "grok-4.6", "model_price"))
}

// A margin multiplication leaves float noise that would be written straight into
// the price table and then reported as a difference on the next run, so the
// rounding is part of the contract rather than cosmetic.
func TestWithMarginRoundsFloatNoise(t *testing.T) {
	assert.Equal(t, 0.096, WithMargin(0.08, 0.2))
	assert.Equal(t, 1.0, WithMargin(1, 0))
	assert.Equal(t, 2.5, WithMargin(2, 0.25))
}

// The margin belongs on the price, not on the multipliers that scale it.
//
// completion_ratio and cache_ratio ride along with model_ratio, so marking them
// up multiplies the margin into the customer's bill: a 5x completion ratio under
// a 20% margin would charge 6x, and a 0.04 cache ratio would stop being a
// discount. Compare and apply must agree, so both go through this helper.
func TestWithMarginForFieldLeavesRatiosAlone(t *testing.T) {
	assert.Equal(t, 1.2, WithMarginForField("model_ratio", 1, 0.2))
	assert.Equal(t, 0.12, WithMarginForField("model_price", 0.1, 0.2))
	assert.Equal(t, 5.0, WithMarginForField("completion_ratio", 5, 0.2))
	assert.Equal(t, 0.04, WithMarginForField("cache_ratio", 0.04, 0.2))
}

// A bulk sync must not rewrite the ratio fields, which is the same rule the
// comparison uses to decide a synced model is healthy.
func TestBuildSyncEditsDoesNotMarkUpRatioFields(t *testing.T) {
	changes := []model.PriceChange{
		{ModelName: "m", Field: "model_ratio", Ours: ptr[float64](0.81), Theirs: ptr[float64](0.675)},
		{ModelName: "m", Field: "completion_ratio", Ours: ptr[float64](5), Theirs: ptr[float64](5)},
		{ModelName: "m", Field: "cache_ratio", Ours: ptr[float64](0.04), Theirs: ptr[float64](0.04)},
	}

	edits, skipped := BuildSyncEdits(changes, 0.2)

	require.Len(t, edits, 3)
	require.Equal(t, 0, skipped)
	byField := map[string]float64{}
	for _, edit := range edits {
		byField[edit.Field] = edit.Value
	}
	assert.Equal(t, 0.81, byField["model_ratio"], "the price carries the margin")
	assert.Equal(t, 5.0, byField["completion_ratio"], "the multiplier is copied as published")
	assert.Equal(t, 0.04, byField["cache_ratio"], "the multiplier is copied as published")
}

// The sentinel reports upstream models we do not carry so the operator knows
// they exist; syncing one would start billing for a model we never chose to
// sell, so it must be counted and left alone.
func TestBuildSyncEditsSkipsModelsWeNeverPriced(t *testing.T) {
	changes := []model.PriceChange{
		{ModelName: "priced", Field: "model_ratio", Ours: ptr[float64](0.5), Theirs: ptr[float64](1)},
		{ModelName: "unknown", Field: "model_ratio", Ours: nil, Theirs: ptr[float64](2)},
		{ModelName: "unpriced", Field: "model_ratio", Ours: ptr[float64](1), Theirs: nil},
	}

	edits, skipped := BuildSyncEdits(changes, 0.2)

	require.Len(t, edits, 1, "only the model we already price may be written")
	assert.Equal(t, "priced", edits[0].ModelName)
	assert.Equal(t, "model_ratio", edits[0].Field)
	assert.Equal(t, 1.2, edits[0].Value, "the write carries the margin")
	assert.Equal(t, 2, skipped)
}

// A rejected entry must leave the table untouched. The writes are grouped by
// dimension and each table is read-modify-written once, so validating lazily
// would let a typo late in a long report land half of it.
func TestApplyPriceEditsRejectsTheWholeBatchBeforeWriting(t *testing.T) {
	cases := []struct {
		name  string
		edits []PriceEdit
	}{
		{
			name: "model name that is only whitespace",
			edits: []PriceEdit{
				{ModelName: "fine", Field: "model_ratio", Value: 1},
				{ModelName: "  ", Field: "model_ratio", Value: 1},
			},
		},
		{
			name: "dimension the page may not write",
			edits: []PriceEdit{
				{ModelName: "fine", Field: "model_ratio", Value: 1},
				{ModelName: "fine", Field: "group_ratio", Value: 1},
			},
		},
		{
			name: "negative price",
			edits: []PriceEdit{
				{ModelName: "fine", Field: "model_ratio", Value: 1},
				{ModelName: "fine", Field: "model_ratio", Value: -1},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			written, err := ApplyPriceEdits(tc.edits)

			require.Error(t, err)
			assert.Equal(t, 0, written, "nothing may be written when the batch is rejected")
		})
	}
}
