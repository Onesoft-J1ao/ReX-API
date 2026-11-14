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
import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  AlertTriangle,
  Info,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import {
  getSuccessRateDotClass,
  getSuccessRateLevel,
  getSuccessRateTextClass,
  formatLatency,
} from '@/features/performance-metrics/lib/format'
import { cn } from '@/lib/utils'

import { getStatusCheck } from '../api'
import type {
  HealthLevel,
  StatusBucket,
  StatusGroup,
  StatusModel,
} from '../types'

// A model with no data yet ranks below every measured one: nothing is known
// about it, so it must not push a failing model out of the preview.
const LEVEL_RANK: Record<HealthLevel | 'unknown', number> = {
  critical: 0,
  warning: 1,
  good: 2,
  excellent: 3,
  unknown: 4,
}

/** Hours of history the strip keeps; the gateway buckets traffic hourly. */
const STRIP_BUCKETS = 24

/**
 * Whether a row counts towards the "N failing" badges.
 *
 * The badge sits next to the same number the row displays, so it has to agree
 * with it. `ok` is the last probe alone: one failed probe next to 377 successful
 * requests left the row showing 82% while the badge called it failing. Only a
 * row that is actually measured and actually below the warning line counts.
 */
function isFailingModel(model: StatusModel): boolean {
  if (model.has_data === false) return false
  if (!Number.isFinite(model.availability)) return false
  return model.availability < 70
}

/**
 * How many model rows a group card shows before collapsing. A group can route
 * dozens of models, and rendering them all makes the page one long scroll, so
 * the card lists the first few and offers the rest behind a toggle. Failing
 * models sort first, so the ones worth seeing are inside the preview.
 */
const MODEL_PREVIEW_COUNT = 6

/**
 * Per-model history strip: one hairline bar per observed hour, newest on the
 * right. Bars are fixed-width so the strip reads as a timeline rather than
 * stretching to fill the row, and the container right-aligns with overflow
 * hidden so a narrow card drops the oldest hours instead of the newest.
 */
function HealthStrip(props: { series?: StatusBucket[] }) {
  const { t } = useTranslation()
  const buckets = props.series?.slice(-STRIP_BUCKETS) ?? []
  if (buckets.length === 0) return null

  return (
    <div
      role='img'
      aria-label={t('Recent availability by hour')}
      className='flex min-w-0 items-end justify-end gap-1 overflow-hidden'
    >
      {buckets.map((bucket) => {
        const idle = bucket.requests === 0 && !bucket.mirrored
        return (
          <span
            key={bucket.ts}
            title={
              idle
                ? t('No traffic this hour')
                : bucket.mirrored
                  ? `${bucket.ok_pct}% · ${t('Upstream mirror')}`
                  : `${bucket.ok_pct}% · ${bucket.requests} · ${formatLatency(bucket.latency_ms)}`
            }
            className={cn(
              'h-4 w-[3px] shrink-0 rounded-full',
              idle ? 'bg-muted' : getSuccessRateDotClass(bucket.ok_pct)
            )}
          />
        )
      })}
    </div>
  )
}

function sortModels(models: StatusModel[]) {
  return [...models].sort((a, b) => {
    const levelDelta = LEVEL_RANK[a.level] - LEVEL_RANK[b.level]
    if (levelDelta !== 0) return levelDelta
    if (a.ok !== b.ok) return a.ok ? 1 : -1
    return a.availability - b.availability
  })
}

function ModelRow(props: { model: StatusModel }) {
  const { model } = props
  const { t } = useTranslation()
  const rate = Math.round(model.availability * 100) / 100
  const errorText = model.error_msg?.trim()
  // A served model with no probe and no traffic in the window: it is listed so
  // the group shows what it offers, but nothing is known about it yet. Showing
  // it as a percentage would invent a measurement.
  const hasData = model.has_data !== false

  return (
    <div className='hover:bg-muted/40 flex flex-col gap-1.5 px-5 py-3 transition-colors sm:px-6'>
      <div className='flex items-start gap-2.5'>
        <span
          className={cn(
            'mt-1.5 size-2 shrink-0 rounded-full',
            hasData ? getSuccessRateDotClass(rate) : 'bg-muted'
          )}
        />
        <span
          className={cn(
            'min-w-0 flex-1 text-sm leading-5 break-words',
            hasData ? 'text-foreground' : 'text-muted-foreground'
          )}
          title={model.model}
        >
          {model.model}
        </span>
        {hasData ? (
          <span
            className={cn(
              'shrink-0 text-right font-mono text-sm font-semibold tabular-nums',
              getSuccessRateTextClass(rate)
            )}
          >
            {Number.isFinite(rate) ? `${rate.toFixed(0)}%` : '—'}
          </span>
        ) : (
          <span className='text-muted-foreground shrink-0 font-mono text-xs'>
            {t('No data')}
          </span>
        )}
      </div>
      {hasData && (
        <div className='flex items-center justify-between gap-3 pl-[1.125rem]'>
          <span className='text-muted-foreground shrink-0 font-mono text-xs tabular-nums'>
            {formatLatency(model.latency_ms)}
          </span>
          <HealthStrip series={model.series} />
        </div>
      )}
      {errorText && !model.ok && (
        <p className='text-destructive pl-[1.125rem] font-mono text-xs leading-5 break-words'>
          {errorText}
        </p>
      )}
      {hasData && !model.has_active && (model.passive_requests ?? 0) > 0 && (
        <p className='text-muted-foreground pl-[1.125rem] font-mono text-xs'>
          {t('Passive traffic only')} · {model.passive_success ?? 0}/
          {model.passive_requests}
        </p>
      )}
    </div>
  )
}

function GroupCard(props: { group: StatusGroup; index: number }) {
  const { group, index } = props
  const { t } = useTranslation()
  const rate = Math.round(group.availability * 100) / 100
  // Absent means an older gateway that never reported it; treat that as observed
  // so the page does not grey out every group after an upgrade.
  const observed = group.observed !== false
  const models = useMemo(() => sortModels(group.models), [group.models])
  // A model with no data is not a failing one, so it must not inflate the count.
  // The test is the availability the row displays, not the probe result: a
  // single failed probe beside hundreds of successful requests is not a failing
  // model, and counting `!ok` reported four of them as down while the same rows
  // showed 50% and above.
  const failed = group.models.filter(isFailingModel).length
  const [showAllModels, setShowAllModels] = useState(false)
  const hiddenModelCount = models.length - MODEL_PREVIEW_COUNT
  const visibleModels = showAllModels
    ? models
    : models.slice(0, MODEL_PREVIEW_COUNT)

  return (
    <Card className='gap-0 overflow-hidden py-0 shadow-xs'>
      <CardHeader className='gap-2 border-b px-5 py-4 sm:px-6'>
        <div className='flex items-start justify-between gap-3'>
          <div className='flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5'>
            <span className='text-muted-foreground font-mono text-xs tabular-nums'>
              {String(index + 1).padStart(2, '0')}
            </span>
            <h3
              className='text-foreground min-w-0 text-sm font-semibold break-words'
              title={group.group}
            >
              {group.group}
            </h3>
            {failed > 0 && (
              <Badge
                variant='destructive'
                className='font-mono tracking-wide uppercase'
              >
                {t('{{count}} failing', { count: failed })}
              </Badge>
            )}
          </div>
          <div className='flex shrink-0 items-center gap-3'>
            {/* An unobserved group reports 100% because nothing contradicts it.
                That is an assumption, not a measurement, so it is greyed to
                keep it from reading as a verified all-green. */}
            <span
              className={cn(
                'font-mono text-lg font-semibold tabular-nums',
                observed
                  ? getSuccessRateTextClass(rate)
                  : 'text-muted-foreground'
              )}
              title={
                observed ? undefined : t('No probes or traffic in this window')
              }
            >
              {Number.isFinite(rate) ? `${rate.toFixed(0)}%` : '—'}
            </span>
            {observed ? (
              <span className='text-muted-foreground hidden font-mono text-xs tabular-nums md:inline'>
                {formatLatency(group.avg_latency_ms)}
              </span>
            ) : (
              <span className='text-muted-foreground hidden font-mono text-xs md:inline'>
                {t('No data')}
              </span>
            )}
          </div>
        </div>
        <div className='text-muted-foreground flex items-center justify-between font-mono text-xs tabular-nums'>
          <span>
            {group.ok_probes}/{group.total_probes} {t('probes ok')}
          </span>
          <span>
            {group.models.length} {t('models')}
          </span>
        </div>
        <div className='bg-muted mt-1 h-1 overflow-hidden rounded-full'>
          <div
            className={cn(
              'h-full rounded-full transition-[width] duration-700 ease-out',
              getSuccessRateDotClass(rate)
            )}
            style={{ width: `${Math.min(100, Math.max(0, rate))}%` }}
          />
        </div>
      </CardHeader>
      <CardContent className='p-0'>
        <div className='divide-border/50 divide-y'>
          {models.length === 0 ? (
            <div className='text-muted-foreground px-5 py-5 text-sm sm:px-6'>
              {t('No models reported')}
            </div>
          ) : (
            visibleModels.map((model) => (
              <ModelRow
                key={`${model.model}-${model.channel_id ?? ''}`}
                model={model}
              />
            ))
          )}
        </div>
        {hiddenModelCount > 0 && (
          <Button
            variant='ghost'
            className='text-muted-foreground hover:text-foreground border-border h-auto w-full rounded-none border-x-0 border-b-0 px-5 py-3 text-sm font-normal sm:px-6'
            onClick={() => setShowAllModels((shown) => !shown)}
            aria-expanded={showAllModels}
          >
            {showAllModels
              ? t('Show fewer models')
              : t('Show {{count}} more models', { count: hiddenModelCount })}
          </Button>
        )}
      </CardContent>
    </Card>
  )
}

export function StatusPanel() {
  const { t } = useTranslation()

  const { data, dataUpdatedAt, isLoading, isError, isFetching, refetch } =
    useQuery({
      queryKey: ['status-check'],
      queryFn: async () => {
        const res = await getStatusCheck()
        return res.data
      },
      refetchInterval: 60_000,
      staleTime: 15_000,
    })

  const announcement = data?.announcement?.trim()
  const groups = data?.groups
  const groupList = groups ?? []
  const hasData = groupList.length > 0
  const summary = useMemo(() => {
    const list = groups ?? []
    const models = list.flatMap((group) => group.models)
    // Same rule as the per-group badge: an unobserved model is not failing, and
    // neither is one whose own availability is healthy.
    const failing = models.filter(isFailingModel).length
    const probes = list.reduce((sum, group) => sum + group.total_probes, 0)
    const okProbes = list.reduce((sum, group) => sum + group.ok_probes, 0)
    // An unobserved group's 100% is an assumption; letting it into the worst-case
    // figure would report the page's best-looking number as its floor.
    const worst = list.reduce<number | null>((min, group) => {
      if (group.observed === false) return min
      if (!Number.isFinite(group.availability)) return min
      if (min === null) return group.availability
      return Math.min(min, group.availability)
    }, null)
    return {
      groups: list.length,
      models: models.length,
      failing,
      probes,
      okProbes,
      worst,
    }
  }, [groups])

  return (
    <div className='mx-auto max-w-7xl space-y-6'>
      <div className='flex items-center justify-between gap-3'>
        <div>
          <h1 className='text-lg font-semibold'>{t('Channel Status')}</h1>
          <p className='text-muted-foreground text-sm'>
            {t('Live health of routed models and groups')}
          </p>
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={() => void refetch()}
          disabled={isFetching}
          className='gap-1.5'
        >
          <RefreshCw className={cn('size-3.5', isFetching && 'animate-spin')} />
          {t('Refresh')}
        </Button>
      </div>

      {announcement && (
        <div className='border-border/60 bg-primary/5 rounded-xl border px-4 py-3 text-sm'>
          {announcement}
        </div>
      )}

      {hasData && (
        <div className='grid gap-4 sm:grid-cols-3'>
          <SummaryTile
            icon={<ShieldCheck className='size-3.5' />}
            label={t('Groups')}
            value={String(summary.groups)}
          />
          <SummaryTile
            icon={<Activity className='size-3.5' />}
            label={t('Probes')}
            value={
              summary.probes > 0 ? `${summary.okProbes}/${summary.probes}` : '—'
            }
          />
          <SummaryTile
            icon={<AlertTriangle className='size-3.5' />}
            label={t('Failing models')}
            value={String(summary.failing)}
            tone={
              summary.failing > 0
                ? 'critical'
                : getSuccessRateLevel(summary.worst ?? 100)
            }
          />
        </div>
      )}

      {dataUpdatedAt > 0 && hasData && (
        <p className='text-muted-foreground font-mono text-xs tabular-nums'>
          {t('Updated')} {new Date(dataUpdatedAt).toLocaleTimeString()}
        </p>
      )}

      {renderState()}
    </div>
  )

  function renderState() {
    if (isLoading) {
      return (
        <div className='text-muted-foreground flex items-center gap-2 py-16 text-sm'>
          <Activity className='size-4 animate-pulse' />
          {t('Loading...')}
        </div>
      )
    }
    if (isError) {
      return (
        <div className='text-muted-foreground bg-card rounded-xl border px-4 py-8 text-center text-sm'>
          {t('Failed to load channel status')}
        </div>
      )
    }
    if (!hasData) {
      return (
        <div className='bg-card rounded-xl border px-5 py-10 text-center sm:px-8'>
          <div className='mb-2 text-3xl'>—</div>
          <div className='text-foreground text-sm font-medium'>
            {t('No status monitoring configured')}
          </div>
          <p className='text-muted-foreground mx-auto mt-2 max-w-md text-xs leading-6'>
            {t(
              'Channel health is collected by a scheduled probe. Enable it in the system configuration to see per-group and per-model availability here.'
            )}
          </p>
        </div>
      )
    }
    return (
      <div className='space-y-4'>
        <div className='grid grid-cols-1 gap-6 md:grid-cols-2 xl:grid-cols-3'>
          {groupList.map((group, idx) => (
            <GroupCard key={group.group} group={group} index={idx} />
          ))}
        </div>
        {groupList.every((g) => g.models.length === 0) && (
          <div className='text-muted-foreground flex items-start gap-2 rounded-xl border border-dashed px-4 py-3 text-xs leading-6'>
            <Info className='mt-0.5 size-3.5 shrink-0' />
            <span>
              {t(
                'Groups are configured, but no model probes have run yet. Enable the channel health probe to populate this page.'
              )}
            </span>
          </div>
        )}
      </div>
    )
  }
}

function toneSampleRate(tone: ReturnType<typeof getSuccessRateLevel>) {
  switch (tone) {
    case 'excellent':
      return 100
    case 'good':
      return 90
    case 'warning':
      return 70
    case 'critical':
      return 0
    default:
      return Number.NaN
  }
}

function SummaryTile(props: {
  icon: ReactNode
  label: string
  value: string
  tone?: ReturnType<typeof getSuccessRateLevel>
}) {
  const valueClass = props.tone
    ? getSuccessRateTextClass(toneSampleRate(props.tone))
    : 'text-foreground'

  return (
    <div className='bg-card rounded-xl border px-4 py-3'>
      <div className='text-muted-foreground flex items-center gap-1.5 text-xs'>
        {props.icon}
        {props.label}
      </div>
      <div
        className={cn(
          'mt-1 font-mono text-lg font-semibold tabular-nums',
          valueClass
        )}
      >
        {props.value}
      </div>
    </div>
  )
}
