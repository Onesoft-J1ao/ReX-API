/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// An override for a group with no group ratio can never price a request, so
// publishing it would show the viewer a figure nothing is ever charged at.
func TestFilterGroupBillingSourceByGroups(t *testing.T) {
	ratios := map[string]float64{"default": 1, "vip": 0.8}

	tests := []struct {
		name    string
		sources map[string]map[string]string
		want    map[string]map[string]string
	}{
		{
			name:    "nothing configured",
			sources: nil,
			want:    nil,
		},
		{
			name:    "a group with a ratio is kept",
			sources: map[string]map[string]string{"vip": {"kimi-k3": "price"}},
			want:    map[string]map[string]string{"vip": {"kimi-k3": "price"}},
		},
		{
			name:    "a group with no ratio is dropped",
			sources: map[string]map[string]string{"gone": {"kimi-k3": "price"}},
			want:    nil,
		},
		{
			// auto is synthesised rather than configured, so it is exempt from
			// the ratio lookup the same way the pricing groups are.
			name:    "auto is exempt",
			sources: map[string]map[string]string{"auto": {"kimi-k3": "ratio"}},
			want:    map[string]map[string]string{"auto": {"kimi-k3": "ratio"}},
		},
		{
			name:    "an empty model map is dropped",
			sources: map[string]map[string]string{"vip": {}},
			want:    nil,
		},
		{
			name: "only the unusable groups are dropped",
			sources: map[string]map[string]string{
				"vip":  {"kimi-k3": "price"},
				"gone": {"kimi-k3": "price"},
			},
			want: map[string]map[string]string{"vip": {"kimi-k3": "price"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, filterGroupBillingSourceByGroups(tc.sources, ratios))
		})
	}
}

// The filter must not mutate the option's own maps — they are the live setting.
func TestFilterGroupBillingSourceDoesNotMutateInput(t *testing.T) {
	sources := map[string]map[string]string{
		"vip":  {"kimi-k3": "price"},
		"gone": {"kimi-k3": "price"},
	}
	filterGroupBillingSourceByGroups(sources, map[string]float64{"default": 1})

	assert.Contains(t, sources, "gone", "filtering must not strip the caller's map")
	assert.Contains(t, sources, "vip")
}