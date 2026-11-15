package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The excluded-id list arrives as a comma-separated query value, so it has to
// tolerate the shapes a hand-typed URL produces without narrowing the query in
// a way the operator did not ask for.
func TestParseExcludedUserIds(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []int
	}{
		{name: "empty means no exclusion", raw: "", want: nil},
		{name: "whitespace only means no exclusion", raw: "   ", want: nil},
		{name: "single id", raw: "7", want: []int{7}},
		{name: "comma separated", raw: "7,12,30", want: []int{7, 12, 30}},
		{name: "spaces around entries", raw: " 7 , 12 ", want: []int{7, 12}},
		{name: "space separated", raw: "7 12 30", want: []int{7, 12, 30}},
		{name: "mixed separators", raw: "7,12 30", want: []int{7, 12, 30}},
		{name: "blank entries are skipped", raw: "7,,12,", want: []int{7, 12}},
		{name: "non-numeric entries are skipped", raw: "7,abc,12", want: []int{7, 12}},
		{name: "zero and negatives are skipped", raw: "0,-3,7", want: []int{7}},
		{name: "all invalid yields no exclusion", raw: "abc,", want: []int{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseExcludedUserIds(tc.raw))
		})
	}
}
