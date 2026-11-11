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

import { resolveModelPerf, type ModelPerfIndex } from './model-health'

type Reading = {
  success_rate: number
  source?: string
  groupScoped?: boolean
  recent_success_series?: { ts: number; success_rate: number }[]
}

function indexOf(parts: {
  probe?: [string, Reading][]
  rollup?: [string, Reading][]
  wide?: [string, Reading][]
}): ModelPerfIndex<Reading> {
  return {
    probeByGroup: new Map(parts.probe ?? []),
    rollupByGroup: new Map(parts.rollup ?? []),
    modelWide: new Map(parts.wide ?? []),
  }
}

describe('resolveModelPerf', () => {
  test('a selected group uses that group reading', () => {
    const index = indexOf({
      rollup: [['ClaudeMax|claude-opus-4-6', { success_rate: 100 }]],
      wide: [['claude-opus-4-6', { success_rate: 65.7, source: 'mirror' }]],
    })
    expect(resolveModelPerf(index, 'claude-opus-4-6', 'ClaudeMax')).toEqual({
      success_rate: 100,
    })
  })

  test('a group with no data borrows the model-wide figure and says so', () => {
    const index = indexOf({
      wide: [['claude-opus-4-6', { success_rate: 65.7 }]],
    })
    const reading = resolveModelPerf(index, 'claude-opus-4-6', 'ClaudeMax2')
    expect(reading?.success_rate).toBe(65.7)
    expect(reading?.groupScoped).toBe(false)
  })

  // The unfiltered square stands for the model as we serve it. Showing the
  // upstream's own model-wide number painted every card red at ~65% while each
  // of our groups read 100%.
  test('the unfiltered square aggregates our groups instead of the mirror', () => {
    const index = indexOf({
      rollup: [
        ['ClaudeMax|claude-opus-4-6', { success_rate: 100 }],
        ['ClaudeR1|claude-opus-4-6', { success_rate: 80 }],
      ],
      wide: [['claude-opus-4-6', { success_rate: 65.7, source: 'mirror' }]],
    })
    const reading = resolveModelPerf(index, 'claude-opus-4-6')
    expect(reading?.success_rate).toBe(90)
    expect(reading?.source).toBeUndefined()
  })

  test('the aggregate averages the series hour by hour', () => {
    const index = indexOf({
      rollup: [
        [
          'A|m',
          {
            success_rate: 100,
            recent_success_series: [
              { ts: 1, success_rate: 100 },
              { ts: 2, success_rate: 100 },
            ],
          },
        ],
        [
          'B|m',
          {
            success_rate: 0,
            recent_success_series: [
              { ts: 2, success_rate: 0 },
              { ts: 3, success_rate: 0 },
            ],
          },
        ],
      ],
      wide: [['m', { success_rate: 10, source: 'mirror' }]],
    })
    const reading = resolveModelPerf(index, 'm')
    expect(reading?.recent_success_series).toEqual([
      { ts: 1, success_rate: 100 },
      { ts: 2, success_rate: 50 },
      { ts: 3, success_rate: 0 },
    ])
  })

  test('a single group is reproduced unchanged', () => {
    const only = { success_rate: 97.5, recent_success_series: [] }
    const index = indexOf({ rollup: [['A|m', only]] })
    expect(resolveModelPerf(index, 'm')).toBe(only)
  })

  test('falls back to the mirror when we have no group reading at all', () => {
    const index = indexOf({
      wide: [['m', { success_rate: 42, source: 'mirror' }]],
    })
    expect(resolveModelPerf(index, 'm')?.success_rate).toBe(42)
  })
})
