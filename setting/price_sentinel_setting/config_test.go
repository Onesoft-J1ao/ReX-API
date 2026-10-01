package price_sentinel_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The markup multiplies every price a sync writes, so a value that arrives
// malformed or out of range has to be bounded at the boundary rather than
// passed through. A zero default would put every synced price onto the
// upstream's own number, which sells at cost.
func TestMarginPercentIsBoundedAndNonZero(t *testing.T) {
	assert.Equal(t, 0.0, ClampMarginPercent(-1))
	assert.Equal(t, MaxMarginPercent, ClampMarginPercent(MaxMarginPercent+1))
	assert.Equal(t, 0.25, ClampMarginPercent(0.25))
	assert.Positive(t, GetMarginPercent())
}
