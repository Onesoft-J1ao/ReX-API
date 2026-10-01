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
import type { PerfMetricsBucketTime } from '@/stores/system-config-store'

import { formatPerfBucketLabel } from './bucket-label'

/** Below this visible span an hourly tick label needs minutes to stay distinct. */
export const NARROW_AXIS_SPAN_MS = 6 * 3_600_000

/** A one-sample series still gets this much axis, so its ticks stay readable. */
const MIN_AXIS_SPAN_MS = 3_600_000

/**
 * Render one time-axis tick, or a tooltip title, as a bucket label.
 *
 * The trend charts plot sample timestamps on a linear axis rather than a
 * categorical one, because the samples are not evenly spaced: the upstream
 * probes some models every few minutes and others hours apart. On a categorical
 * axis those real gaps are flattened into equal steps and samples that share a
 * label collapse onto one tick, which makes the trend shape wrong. A linear
 * axis keeps the true spacing but hands the raw timestamp to the formatter, so
 * this converts whatever VChart passes back into a millisecond value. The
 * compact YYYYMMDD form is accepted too, because VChart may pre-format a tick
 * before calling.
 */
export function formatTimeAxisValue(
  value: unknown,
  bucket: PerfMetricsBucketTime,
  spanMs: number
): string {
  let ms: number
  if (value instanceof Date) {
    ms = value.getTime()
  } else if (typeof value === 'number') {
    ms = value
  } else {
    const compactDate = /^(\d{4})(\d{2})(\d{2})$/.exec(String(value))
    ms = compactDate
      ? new Date(
          Number(compactDate[1]),
          Number(compactDate[2]) - 1,
          Number(compactDate[3])
        ).getTime()
      : Date.parse(String(value))
  }
  if (!Number.isFinite(ms)) return String(value)
  // An hourly bucket labels without minutes, which is right for a day-wide axis
  // but makes every tick of a short axis read the same.
  const effectiveBucket =
    bucket === 'hour' && spanMs < NARROW_AXIS_SPAN_MS ? 'minute' : bucket
  return formatPerfBucketLabel(ms, effectiveBucket)
}

/**
 * The linear domain for a set of sample timestamps, padded so the first and last
 * point do not sit flush against the frame. VChart does not derive this domain
 * from the data on this axis, so it is computed here, and a minimum span keeps a
 * one-sample series from collapsing into a millisecond-wide axis.
 */
export function timeAxisDomain(
  timestamps: number[],
  minSpanMs = MIN_AXIS_SPAN_MS
): { min: number; max: number } {
  let min = Math.min(...timestamps)
  let max = Math.max(...timestamps)
  const span = max - min
  if (span < minSpanMs) {
    const grow = (minSpanMs - span) / 2
    min -= grow
    max += grow
  }
  const pad = Math.max(1, Math.round((max - min) * 0.02))
  return { min: min - pad, max: max + pad }
}
