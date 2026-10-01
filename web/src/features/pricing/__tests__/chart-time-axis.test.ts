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

import { formatPerfBucketLabel } from '../lib/bucket-label'
import {
  formatTimeAxisValue,
  NARROW_AXIS_SPAN_MS,
  timeAxisDomain,
} from '../lib/chart-time-axis'

const HOUR_MS = 3_600_000

describe('timeAxisDomain', () => {
  it('keeps a one-sample series on a readable span instead of a millisecond one', () => {
    const ts = Date.UTC(2026, 8, 27, 6, 30)

    const domain = timeAxisDomain([ts])

    expect(domain.min).toBeLessThan(ts)
    expect(domain.max).toBeGreaterThan(ts)
    expect(domain.max - domain.min).toBeGreaterThanOrEqual(HOUR_MS)
  })

  it('brackets the first and last sample so neither sits on the frame edge', () => {
    const first = Date.UTC(2026, 8, 27, 0, 0)
    const last = first + 4 * HOUR_MS

    const domain = timeAxisDomain([first, last])

    expect(domain.min).toBeLessThan(first)
    expect(domain.max).toBeGreaterThan(last)
  })

  it('does not shrink a span that is already wide enough', () => {
    const first = Date.UTC(2026, 8, 27, 0, 0)
    const last = first + 23 * HOUR_MS

    const domain = timeAxisDomain([first, last])

    expect(domain.max - domain.min).toBeGreaterThanOrEqual(23 * HOUR_MS)
  })
})

describe('formatTimeAxisValue', () => {
  const ts = Date.UTC(2026, 8, 27, 6, 30)

  it('labels a wide axis with the hourly bucket', () => {
    expect(formatTimeAxisValue(ts, 'hour', 24 * HOUR_MS)).toBe(
      formatPerfBucketLabel(ts, 'hour')
    )
  })

  it('adds minutes on a narrow axis, where hourly labels would all read the same', () => {
    const narrowLabel = formatTimeAxisValue(ts, 'hour', HOUR_MS)

    expect(narrowLabel).toBe(formatPerfBucketLabel(ts, 'minute'))
    expect(narrowLabel).not.toBe(formatPerfBucketLabel(ts, 'hour'))
  })

  it('treats the threshold itself as wide enough', () => {
    expect(formatTimeAxisValue(ts, 'hour', NARROW_AXIS_SPAN_MS)).toBe(
      formatPerfBucketLabel(ts, 'hour')
    )
  })

  it('leaves a non-hourly bucket alone at any span', () => {
    expect(formatTimeAxisValue(ts, 'minute', 24 * HOUR_MS)).toBe(
      formatPerfBucketLabel(ts, 'minute')
    )
  })

  it('reads the same timestamp given as a number or a Date', () => {
    expect(formatTimeAxisValue(new Date(ts), 'hour', 24 * HOUR_MS)).toBe(
      formatTimeAxisValue(ts, 'hour', 24 * HOUR_MS)
    )
  })

  it('accepts the compact YYYYMMDD form a time axis may hand over', () => {
    const midnight = new Date(2026, 8, 26).getTime()

    expect(formatTimeAxisValue('20260926', 'hour', 24 * HOUR_MS)).toBe(
      formatPerfBucketLabel(midnight, 'hour')
    )
  })

  it('falls back to the raw value when nothing can be parsed', () => {
    expect(formatTimeAxisValue('not-a-time', 'hour', 24 * HOUR_MS)).toBe(
      'not-a-time'
    )
  })
})
