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
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import '@/i18n/config'

import { ModelPerfBadge } from '../model-perf-badge'

const SLOT_COUNT = 6

function stripOf(container: HTMLElement) {
  const strip = container.querySelector('div.flex.h-4.items-center.justify-end')
  expect(strip).not.toBeNull()
  return strip as HTMLElement
}

function barsOf(container: HTMLElement) {
  return Array.from(stripOf(container).querySelectorAll('span'))
}

describe('ModelPerfBadge status strip', () => {
  it('renders one bar per recent rate once the window is full', () => {
    const { container } = render(
      <ModelPerfBadge
        perf={{
          avg_latency_ms: 1200,
          avg_tps: 30,
          success_rate: 98.5,
          recent_success_rates: [90, 95, 99, 100, 97, 88],
        }}
      />
    )
    expect(barsOf(container)).toHaveLength(SLOT_COUNT)
  })

  it('keeps the slot count when only a few buckets have data', () => {
    const { container } = render(
      <ModelPerfBadge
        perf={{
          avg_latency_ms: 1200,
          avg_tps: 30,
          success_rate: 98.5,
          recent_success_rates: [99, 98],
        }}
      />
    )
    // Short series pad at the front, so the timeline stays newest-on-the-right.
    expect(barsOf(container)).toHaveLength(SLOT_COUNT)
  })

  it('truncates to the newest slots when the backend returns more', () => {
    const { container } = render(
      <ModelPerfBadge
        perf={{
          avg_latency_ms: 1200,
          avg_tps: 30,
          success_rate: 98.5,
          recent_success_rates: [1, 2, 3, 4, 5, 6, 7, 8],
        }}
      />
    )
    expect(barsOf(container)).toHaveLength(SLOT_COUNT)
  })

  it('falls back to the overall rate when no series is available', () => {
    const { container } = render(
      <ModelPerfBadge
        perf={{ avg_latency_ms: 1200, avg_tps: 30, success_rate: 42 }}
      />
    )
    expect(barsOf(container)).toHaveLength(SLOT_COUNT)
  })

  it('renders the bars in ascending height so the strip reads as a timeline', () => {
    const { container } = render(
      <ModelPerfBadge
        perf={{
          avg_latency_ms: 1200,
          avg_tps: 30,
          success_rate: 98.5,
          recent_success_rates: [90, 95, 99, 100, 97, 88],
        }}
      />
    )
    const heights = barsOf(container).map(
      (bar) => bar.className.match(/\bh-[\d.]+/)?.[0]
    )
    expect(heights).toEqual(['h-1.5', 'h-2', 'h-2.5', 'h-3', 'h-3.5', 'h-4'])
  })

  it('renders nothing without perf data', () => {
    const { container } = render(<ModelPerfBadge perf={undefined} />)
    expect(container).toBeEmptyDOMElement()
  })
})
