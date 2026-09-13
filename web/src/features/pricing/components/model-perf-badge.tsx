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
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { getSuccessRateDotClass } from '@/features/performance-metrics/lib/format'
import { cn } from '@/lib/utils'

export type ModelPerfBadgeData = {
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
  recent_success_rates?: number[]
}

export interface ModelPerfBadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  perf: ModelPerfBadgeData | undefined
}

/**
 * How many recent buckets the status strip shows. Keep in sync with
 * recentSuccessRateSlots in pkg/perf_metrics/metrics.go, which decides how
 * many rates the summary endpoint actually returns.
 */
const STATUS_SLOT_COUNT = 6

// Oldest to newest, each step one notch taller so the strip reads as a
// timeline. Sized to fit the h-4 strip.
const STATUS_BAR_HEIGHTS = [
  'h-1.5',
  'h-2',
  'h-2.5',
  'h-3',
  'h-3.5',
  'h-4',
] as const

function formatCompactNumber(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '—'
  return value > 1 ? String(Math.round(value)) : value.toFixed(1)
}

function formatCompactLatency(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '—'
  if (ms >= 1_000) return `${formatCompactNumber(ms / 1_000)}s`
  return `${formatCompactNumber(ms)}ms`
}

function formatCompactThroughput(tps: number): string {
  if (!Number.isFinite(tps) || tps <= 0) return '—'
  if (tps >= 1_000) return `${formatCompactNumber(tps / 1_000)}Kt`
  return `${formatCompactNumber(tps)}t`
}

export const ModelPerfBadge = memo(function ModelPerfBadge(
  props: ModelPerfBadgeProps
) {
  const { t } = useTranslation()

  if (!props.perf) {
    return null
  }

  const { avg_latency_ms, avg_tps, success_rate } = props.perf

  const recentRates =
    props.perf.recent_success_rates?.filter((rate) => Number.isFinite(rate)) ??
    []
  const statusRates =
    recentRates.length > 0
      ? recentRates.slice(-STATUS_SLOT_COUNT)
      : [success_rate]
  const statusBars = [
    ...Array(Math.max(0, STATUS_SLOT_COUNT - statusRates.length)).fill(null),
    ...statusRates,
  ].slice(-STATUS_SLOT_COUNT)

  return (
    <div
      className={cn(
        'hidden w-[140px] grid-cols-[38px_48px_38px] gap-x-2 text-right tabular-nums min-[460px]:grid',
        props.className
      )}
    >
      <div title={t('Average latency')} className='min-w-0'>
        <div className='text-muted-foreground text-xs leading-4'>
          {t('Latency short')}
        </div>
        <div className='text-muted-foreground font-mono text-xs leading-4 tabular-nums whitespace-nowrap'>
          {formatCompactLatency(avg_latency_ms)}
        </div>
      </div>
      <div title={t('Throughput')} className='min-w-0'>
        <div className='text-muted-foreground truncate text-xs leading-4'>
          {t('Throughput short')}
        </div>
        <div className='text-muted-foreground font-mono text-xs leading-4 tabular-nums whitespace-nowrap'>
          {formatCompactThroughput(avg_tps)}
        </div>
      </div>
      <div
        title={`${t('Success rate')}: ${success_rate.toFixed(1)}%`}
        className='min-w-0'
      >
        <div className='text-muted-foreground truncate text-xs leading-4'>
          {t('Status short')}
        </div>
        <div className='flex h-4 items-center justify-end gap-0.5'>
          {statusBars.map((rate, index) => (
            <span
              key={`${index}-${rate ?? 'empty'}`}
              className={cn(
                'w-1 rounded-full',
                STATUS_BAR_HEIGHTS[index] ?? 'h-4',
                rate == null
                  ? index === 0
                    ? 'bg-muted-foreground/10'
                    : 'bg-muted-foreground/15'
                  : getSuccessRateDotClass(rate)
              )}
            />
          ))}
        </div>
      </div>
    </div>
  )
})
