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
import { memo, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  formatLatency,
  formatThroughput,
  getSuccessRateDotClass,
} from '@/features/performance-metrics/lib/format'
import type { SuccessRatePoint } from '@/features/performance-metrics/types'
import { cn } from '@/lib/utils'

export type ModelPerfBadgeData = {
  window_start?: number
  window_end?: number
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
  recent_success_series?: SuccessRatePoint[]
  recent_latency_series?: { ts: number; latency_ms: number }[]
  /**
   * Where the numbers came from. `mirror` means another deployment's aggregate,
   * which has no group dimension — it describes that instance's traffic for the
   * model, not ours and not the group on screen. The badge marks it so a figure
   * we never measured is not mistaken for a measurement of this group.
   */
  source?: string
}

export interface ModelPerfBadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  perf: ModelPerfBadgeData | undefined
}

const STATUS_SLOTS = Array.from({ length: 24 }, (_, slot) => slot)

export const ModelPerfBadge = memo(function ModelPerfBadge(
  props: ModelPerfBadgeProps
) {
  const { t } = useTranslation()
  const latencyText = formatLatency(props.perf?.avg_latency_ms ?? 0)
  const throughputText = formatThroughput(props.perf?.avg_tps ?? 0).replace(
    ' t/s',
    't/s'
  )
  const successRate = props.perf?.success_rate
  const hasSuccessRate =
    successRate != null &&
    Number.isFinite(successRate) &&
    successRate >= 0 &&
    successRate <= 100
  // Another deployment's blended figure. It has no group dimension, so it cannot
  // answer "how is this model doing in this group" — only "how is it doing
  // somewhere". The number stays useful, but it says where it came from.
  const mirrored = props.perf?.source === 'mirror'
  // Hourly points use the server window, including the current partial hour.
  // Hours without traffic stay gray. Slot 23 is the current, partial hour.
  const statusRates = useMemo(() => {
    const windowStart = props.perf?.window_start
    if (windowStart == null) return STATUS_SLOTS.map(() => undefined)
    const ratesByHour = new Map<number, number>()
    for (const point of props.perf?.recent_success_series ?? []) {
      ratesByHour.set(point.ts, point.success_rate)
    }
    return STATUS_SLOTS.map((slot) => {
      const hourStart = windowStart + slot * 3600
      return ratesByHour.get(hourStart)
    })
  }, [props.perf?.recent_success_series, props.perf?.window_start])

  return (
    <div
      aria-label={t('Performance metrics for the last 24 hours')}
      className={cn(
        'flex w-full min-w-0 items-center justify-between gap-3',
        props.className
      )}
    >
      <dl className='flex min-w-0 items-end gap-7 text-xs tabular-nums'>
        <div className='w-20 shrink-0'>
          <dt
            title={t(
              'Success rate excludes business rejections and includes the current partial hour.'
            )}
            className='text-muted-foreground flex items-center justify-between gap-1 text-[13px] leading-4'
          >
            <span title={mirrored ? t('Measured by the mirrored upstream for the model, not for this group') : undefined}>
              {t('Status')}
              {mirrored && <span aria-hidden>*</span>}
            </span>
            <span
              className='font-mono'
              title={mirrored ? t('Mirrored upstream figure for the whole model; this group has no measurement of its own') : undefined}
            >
              {hasSuccessRate ? `${successRate.toFixed(2)}%` : '—'}
            </span>
          </dt>
          <dd
            role='img'
            aria-label={t(
              'Recent success-rate samples; gray bars indicate missing data.'
            )}
            title={t(
              'Recent success-rate samples; gray bars indicate missing data.'
            )}
            className='mt-0.5 flex h-2.5 w-20 items-center gap-px'
          >
            {STATUS_SLOTS.map((slot) => {
              const rate = statusRates[slot]
              return (
                <span
                  key={slot}
                  aria-hidden
                  className={cn(
                    'h-full w-[3px] shrink-0 rounded-full',
                    rate != null &&
                      Number.isFinite(rate) &&
                      rate >= 0 &&
                      rate <= 100
                      ? getSuccessRateDotClass(rate)
                      : 'bg-muted-foreground/15'
                  )}
                />
              )
            })}
          </dd>
        </div>
        <div title={t('Average latency')} className='shrink-0'>
          <dt className='text-muted-foreground text-[13px] leading-4'>
            {t('Latency short')}
          </dt>
          <dd className='mt-0.5 font-mono text-[13px] whitespace-nowrap'>
            {latencyText === '—' ? '—s' : latencyText}
          </dd>
        </div>
        <div title={t('Throughput')} className='shrink-0'>
          <dt className='text-muted-foreground text-[13px] leading-4'>
            {t('Throughput short')}
          </dt>
          <dd className='mt-0.5 font-mono text-[13px] whitespace-nowrap'>
            {throughputText === '—' ? '—t/s' : throughputText}
          </dd>
        </div>
      </dl>
      {props.children}
    </div>
  )
})
