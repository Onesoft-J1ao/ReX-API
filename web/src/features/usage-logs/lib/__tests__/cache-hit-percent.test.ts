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
import { describe, expect, test } from 'vitest'

import { getCacheHitPercent } from '../format'

// Numbers taken from real log rows so the regression is anchored to what the
// gateway actually stores rather than to a guessed shape.
describe('getCacheHitPercent', () => {
  test('reads a DeepSeek row where prompt_tokens already includes the cache hit', () => {
    // prompt_tokens 23,384 already contains the 23,168 cached tokens.
    expect(getCacheHitPercent(23384, 23168)).toBeCloseTo(99.08, 2)
  })

  test('reads an Anthropic row normalised to the same total', () => {
    // Three real rows all land on ~78%, which only holds when prompt_tokens is
    // the whole prompt rather than the uncached remainder.
    expect(getCacheHitPercent(897, 700)).toBeCloseTo(78.04, 2)
    expect(getCacheHitPercent(601, 469)).toBeCloseTo(78.04, 2)
  })

  test('matches the usage-log cache read percentage and keeps writes separate', () => {
    // Reference row: 4,768 / 5,080 = 93.86%; 161 cache-write tokens render separately.
    expect(getCacheHitPercent(5080, 4768)).toBeCloseTo(93.86, 2)
    expect(getCacheHitPercent(1000, 300)).toBeCloseTo(30, 6)
  })

  test('returns zero rather than dividing by zero', () => {
    expect(getCacheHitPercent(0, 0)).toBe(0)
    expect(getCacheHitPercent(0, 100)).toBe(0)
  })

  test('never exceeds one hundred percent', () => {
    expect(getCacheHitPercent(1000, 1000)).toBeCloseTo(100, 6)
    expect(getCacheHitPercent(1000, 1200)).toBeCloseTo(100, 6)
  })
})
