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
  /**
   * False when the card fell back to the model-wide rollup because this group
   * carried no traffic for the model. The number still describes the model, but
   * it is not this group's measurement — and without saying so, two groups that
   * both lack data show what looks like identical health.
   */
  groupScoped?: boolean
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
  const mirrored = props.perf?.source?.startsWith('mirror') === true
  // The card fell back to the model-wide rollup because this group had no
  // traffic for the model. The figure is real, but it is the model's, not this
  // group's — which is why two groups without data otherwise look identical.
  const borrowed = props.perf?.groupScoped === false
  // Hourly points use the server window, including the current partial hour.
  //
  // `window_start` comes from the perf summary or the status check. A reading
  // built from the status check carries its own points but not always that
  // origin — an older gateway omits it — and requiring it blanked the strip even
  // though the points were there. Buckets sit on hour boundaries, so the
  // earliest point is a valid origin when the server does not name one.
  const statusRates = useMemo(() => {
    const points = props.perf?.recent_success_series ?? []
    const earliest = points.reduce<number | undefined>(
      (min, point) => (min == null || point.ts < min ? point.ts : min),
      undefined
    )
    const windowStart = props.perf?.window_start ?? earliest
    if (windowStart == null) return STATUS_SLOTS.map(() => undefined)
    const ratesByHour = new Map<number, number>()
    for (const point of points) {
      ratesByHour.set(point.ts, point.success_rate)
    }
    return STATUS_SLOTS.map((slot) => {
      const hourStart = windowStart + slot * 3600
      return ratesByHour.get(hourStart)
    })
  }, [props.perf?.recent_success_series, props.perf?.window_start])

  // Only hours that were actually observed say anything. A model that answered
  // for one hour is not "0% healthy for the day" — nothing is known about the
  // other 23 — so the badge averages the hours it has and names that count.
  const observedHours = statusRates.filter(
    (rate): rate is number => rate != null && Number.isFinite(rate)
  ).length
  const partialDay = observedHours > 0 && observedHours < STATUS_SLOTS.length
  const sampledRate =
    observedHours > 0
      ? statusRates.reduce<number>((sum, rate) => sum + (rate ?? 0), 0) /
        observedHours
      : undefined
  // A partly sampled day is described by the hours that were sampled: the
  // server's figure spans the whole window, and pairing it with a strip of
  // mostly empty slots reads as "0% while every visible bar is red". A fully
  // sampled day, or one with no hourly points at all, keeps the server figure.
  let shownRate: number | undefined
  if (partialDay) {
    shownRate = sampledRate
  } else if (hasSuccessRate) {
    shownRate = successRate
  } else {
    shownRate = sampledRate
  }
  const hasShownRate =
    shownRate != null &&
    Number.isFinite(shownRate) &&
    shownRate >= 0 &&
    shownRate <= 100
  const shownRateText = hasShownRate ? `${shownRate?.toFixed(2)}%` : '—'

  // Why the percentage is not simply the server's window figure, when it is not.
  let rateTitle: string | undefined
  if (partialDay) {
    rateTitle = t(
      'Average over the {{observed}} of {{total}} hours with a measurement; the rest of the day has none.',
      { observed: observedHours, total: STATUS_SLOTS.length }
    )
  } else if (mirrored) {
    rateTitle = t(
      'Mirrored upstream figure for the whole model; this group has no measurement of its own'
    )
  } else if (borrowed) {
    rateTitle = t(
      'Measured for the model as a whole; this group carried no traffic for it'
    )
  }
  const stripTitle = partialDay
    ? t(
        'Success-rate samples for the last 24 hours; {{observed}} of {{total}} hours measured, gray bars have none.',
        { observed: observedHours, total: STATUS_SLOTS.length }
      )
    : t('Recent success-rate samples; gray bars indicate missing data.')

  // Says whose measurement the Status figure is, when it is not this group's.
  let statusLabelTitle: string | undefined
  if (mirrored) {
    statusLabelTitle = t(
      'Measured by the mirrored upstream for the model, not for this group'
    )
  } else if (borrowed) {
    statusLabelTitle = t(
      'Measured for the model as a whole; this group carried no traffic for it'
    )
  }

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
            <span title={statusLabelTitle}>
              {t('Status')}
              {mirrored && <span aria-hidden>*</span>}
              {borrowed && (
                <span aria-hidden className='ml-0.5'>
                  †
                </span>
              )}
            </span>
            <span
              className='font-mono'
              title={rateTitle}
            >
              {shownRateText}
            </span>
          </dt>
          <dd
            role='img'
            aria-label={t(
              'Recent success-rate samples; gray bars indicate missing data.'
            )}
            title={stripTitle}
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
