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
import { describe, expect, it } from 'vitest'

import type { PerformanceSeriesPoint } from '@/features/performance-metrics/types'

import { toGroupUptimeSeries, toUptimeSeries } from '../lib/uptime-series'

function point(ts: number, successRate: number): PerformanceSeriesPoint {
  return {
    ts,
    avg_ttft_ms: 0,
    avg_latency_ms: 100,
    success_rate: successRate,
    avg_tps: 0,
  }
}

// The detail card reports "N outage hours in the last 24 hours". Counting every
// hour below 100% made a 99.9% hour — one failure in a thousand — an incident,
// so a model that never stopped answering still advertised a dozen of them.
describe('uptime series outage hours', () => {
  it('counts only an hour with no successful request', () => {
    const series = toUptimeSeries([
      point(1, 100),
      point(2, 99.9),
      point(3, 55),
      point(4, 0),
    ])

    expect(series.map((p) => p.incidents)).toEqual([0, 0, 0, 1])
  })

  it('keeps the per-hour success rate as measured', () => {
    const series = toUptimeSeries([point(1, 42.5)])

    expect(series[0].uptime_pct).toBeCloseTo(42.5, 5)
  })

  it('applies the same rule to the per-group strip', () => {
    const series = toGroupUptimeSeries({
      group: 'ClaudeMax',
      avg_ttft_ms: 0,
      avg_latency_ms: 100,
      success_rate: 50,
      avg_tps: 0,
      series: [point(1, 100), point(2, 50), point(3, 0)],
    })

    expect(series.map((p) => p.incidents)).toEqual([0, 0, 1])
  })
})
