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
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'
import { getStatusCheck } from '@/features/status/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  DEFAULT_PRICING_PAGE_SIZE,
  DEFAULT_TOKEN_UNIT,
  FILTER_ALL,
  SORT_OPTIONS,
} from '../constants'
import { modelFamilyRank } from '../lib/filters'
import type { GroupPricingOverrides } from '../lib/price'
import type { PricingModel, TokenUnit } from '../types'
import { ModelCard } from './model-card'
import type { ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardGridProps {
  models: PricingModel[]
  onModelClick: (modelName: string) => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  /** Per-group pricing source overrides; absent when none are configured. */
  groupBillingSource?: Record<string, Record<string, string>>
  /** Per-group per-model token overrides; absent when none are configured. */
  groupPricingOverrides?: GroupPricingOverrides
  sortBy?: string
  pageSize?: number
}

export function ModelCardGrid(props: ModelCardGridProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const pageSize = props.pageSize ?? DEFAULT_PRICING_PAGE_SIZE
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT

  const perfQuery = useQuery({
    queryKey: ['perf-metrics-summary', 24],
    queryFn: async () => requireServerSuccess(await getPerfMetricsSummary(24)),
    staleTime: 60 * 1000,
    retry: false,
  })

  // Active probe health. The endpoint already merges the passive rollups, so it
  // is the one source that has a reading for both kinds of traffic: a model with
  // real requests but no recent probe, and a probe that found an idle model up.
  const healthQuery = useQuery({
    queryKey: ['status-check'],
    queryFn: async () => requireServerSuccess(await getStatusCheck()),
    staleTime: 60 * 1000,
    retry: false,
  })

  // Health is per (model, group). A model is usually served by several groups
  // and they can disagree completely — one group 97% and another 0% — so a map
  // keyed on the model name alone shows one group's numbers on every card.
  // The key keeps the group, and the lookup below picks the group the viewer is
  // looking at.
  const healthByModelAndGroup = useMemo(() => {
    const map = new Map<string, ModelPerfBadgeData>()
    for (const group of healthQuery.data?.data?.groups ?? []) {
      for (const model of group.models ?? []) {
        // Only rows that actually carry a reading belong in the map: a served
        // model with no probe and no traffic is `has_data === false`, and
        // mapping it would paint an empty badge where there is no series. A
        // passive-only row (has_active false but has_data true) is real traffic
        // and must be kept, or a group whose models are all passive shows no
        // health at all.
        if (!model.model || model.has_data === false) continue
        map.set(`${group.group}|${model.model}`, {
          success_rate: model.availability,
          avg_latency_ms: model.latency_ms,
          // The probe does not measure throughput. Reporting the previous
          // group's figure here — which is what carrying it over amounted to —
          // would put one group's TPS on another group's card.
          avg_tps: 0,
          recent_success_series: model.series?.map((bucket) => ({
            ts: bucket.ts,
            success_rate: bucket.ok_pct,
          })),
        })
      }
    }
    return map
  }, [healthQuery.data])

  // The passive rollup split by group. `models` holds a model-wide figure that
  // blends every group the model is routed through — using it for a group card
  // is what made ClaudeMax and Claude企业池 show one identical number.
  // `group_models` keeps them apart; the model-wide map stays for a group this
  // deployment carried no traffic for, and for gateways that do not send it.
  const perfByGroupAndModel = useMemo(() => {
    const map = new Map<string, ModelPerfBadgeData>()
    for (const model of perfQuery.data?.data?.group_models ?? []) {
      if (!model.group || !model.model_name) continue
      map.set(`${model.group}|${model.model_name}`, {
        ...model,
        window_start: perfQuery.data?.data.window_start,
        window_end: perfQuery.data?.data.window_end,
      })
    }
    return map
  }, [perfQuery.data])

  const perfMap = useMemo(() => {
    const map = new Map<string, ModelPerfBadgeData>()
    for (const model of perfQuery.data?.data?.models ?? []) {
      if (!model.model_name) continue
      map.set(model.model_name, {
        ...model,
        window_start: perfQuery.data?.data.window_start,
        window_end: perfQuery.data?.data.window_end,
      })
    }
    return map
  }, [perfQuery.data])

  const perfFor = useCallback(
    (modelName: string): ModelPerfBadgeData | undefined => {
      if (!modelName) return undefined
      const group = props.selectedGroup
      if (group && group !== FILTER_ALL) {
        // In order: this group's probe reading (freshest), then this group's own
        // passive rollup. The model-wide figure comes last, and only when it was
        // measured here — a `mirror` entry is another deployment's blend with no
        // group dimension, so it must not be presented as this group's reading.
        const local =
          healthByModelAndGroup.get(`${group}|${modelName}`) ??
          perfByGroupAndModel.get(`${group}|${modelName}`)
        if (local) return local
        const wide = perfMap.get(modelName)
        return wide?.source === 'mirror' ? undefined : wide
      }
      // No group filter: the card stands for the model as a whole, so any
      // group's reading describes something real about it. The probe is the
      // fresher signal, so it wins over the passive rollup.
      for (const [key, value] of healthByModelAndGroup) {
        if (key.endsWith(`|${modelName}`)) return value
      }
      return perfMap.get(modelName)
    },
    [healthByModelAndGroup, perfByGroupAndModel, perfMap, props.selectedGroup]
  )

  const orderedModels = useMemo(() => {
    if (props.sortBy !== SORT_OPTIONS.RECOMMENDED) return props.models
    // Recommended order surfaces what a buyer can actually judge first:
    //   1. Models with a live reading ahead of models with none. A card with no
    //      health data tells the viewer nothing about whether it works, so it
    //      must not push a measured model down the page.
    //   2. Among models that both have (or both lack) a reading, preferred
    //      families first.
    //   3. Then higher availability first. The comparison falls back to zero for
    //      a model with no sample, and the sort is stable, so ties and no-data
    //      models keep their incoming order.
    return [...props.models].sort((a, b) => {
      const aPerf = perfFor(a.model_name)
      const bPerf = perfFor(b.model_name)
      const aHasHealth = aPerf?.success_rate != null
      const bHasHealth = bPerf?.success_rate != null
      if (aHasHealth !== bHasHealth) return aHasHealth ? -1 : 1
      const family =
        modelFamilyRank(a.model_name) - modelFamilyRank(b.model_name)
      if (family !== 0) return family
      return (bPerf?.success_rate ?? 0) - (aPerf?.success_rate ?? 0)
    })
  }, [props.models, props.sortBy, perfFor])

  const totalPages = Math.max(1, Math.ceil(orderedModels.length / pageSize))
  const currentPage = Math.min(page, totalPages)

  const pagedModels = useMemo(() => {
    const start = (currentPage - 1) * pageSize
    return orderedModels.slice(start, start + pageSize)
  }, [currentPage, pageSize, orderedModels])

  if (props.models.length === 0) {
    return null
  }

  return (
    <div className='flex flex-col gap-4 sm:gap-5'>
      <div className='grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3'>
        {pagedModels.map((model) => (
          <ModelCard
            key={model.id ?? model.model_name}
            model={model}
            tokenUnit={tokenUnit}
            priceRate={props.priceRate}
            usdExchangeRate={props.usdExchangeRate}
            showRechargePrice={props.showRechargePrice}
            selectedGroup={props.selectedGroup}
            groupBillingSource={props.groupBillingSource}
            groupPricingOverrides={props.groupPricingOverrides}
            perf={perfFor(model.model_name)}
            onClick={() => props.onModelClick(model.model_name || '')}
          />
        ))}
      </div>

      {totalPages > 1 && (
        <div className='text-muted-foreground flex flex-col items-center justify-between gap-3 border-t px-4 py-3 text-sm sm:flex-row'>
          <p className='text-muted-foreground'>
            {t('Page {{current}} of {{total}}', {
              current: currentPage,
              total: totalPages,
            })}
          </p>
          <div className='flex items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setPage((current) => Math.max(1, current - 1))}
              disabled={currentPage <= 1}
              className='gap-1.5'
            >
              <ChevronLeft className='size-4' />
              {t('Previous page')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() =>
                setPage((current) => Math.min(totalPages, current + 1))
              }
              disabled={currentPage >= totalPages}
              className='gap-1.5'
            >
              {t('Next page')}
              <ChevronRight className='size-4' />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
