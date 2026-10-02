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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'
import { getStatusCheck } from '@/features/status/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  DEFAULT_PRICING_PAGE_SIZE,
  DEFAULT_TOKEN_UNIT,
  SORT_OPTIONS,
} from '../constants'
import { modelFamilyRank } from '../lib/filters'
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

  const perfMap = useMemo(() => {
    const map = new Map<string, ModelPerfBadgeData>()
    for (const group of healthQuery.data?.data?.groups ?? []) {
      for (const model of group.models ?? []) {
        if (!model.model || !model.has_active) continue
        const existing = map.get(model.model)
        // A model can be served by several groups; keep the healthiest reading
        // instead of letting whichever group is listed last win.
        if (existing && existing.success_rate >= model.availability) continue
        map.set(model.model, {
          success_rate: model.availability,
          avg_latency_ms: model.latency_ms,
          // The probe does not measure throughput, so keep whatever the passive
          // rollup had rather than reporting a made-up zero.
          avg_tps: existing?.avg_tps ?? 0,
          recent_success_series:
            model.series?.map((bucket) => ({
              ts: bucket.ts,
              success_rate: bucket.ok_pct,
            })) ?? existing?.recent_success_series,
        })
      }
    }
    // Traffic with no probe result yet still deserves a reading. The probe is
    // the fresher signal, so a model it already reported keeps that reading.
    for (const model of perfQuery.data?.data?.models ?? []) {
      if (!model.model_name || map.has(model.model_name)) continue
      map.set(model.model_name, {
        ...model,
        window_start: perfQuery.data?.data.window_start,
        window_end: perfQuery.data?.data.window_end,
      })
    }
    return map
  }, [perfQuery.data, healthQuery.data])

  const orderedModels = useMemo(() => {
    if (props.sortBy !== SORT_OPTIONS.RECOMMENDED) return props.models
    // Family first so a healthy model of a later family cannot jump the queue,
    // then availability. The comparison falls back to zero for a model with no
    // sample, and the sort is stable, so those keep their incoming order.
    return [...props.models].sort((a, b) => {
      const family =
        modelFamilyRank(a.model_name) - modelFamilyRank(b.model_name)
      if (family !== 0) return family
      return (
        (perfMap.get(b.model_name || '')?.success_rate ?? 0) -
        (perfMap.get(a.model_name || '')?.success_rate ?? 0)
      )
    })
  }, [props.models, props.sortBy, perfMap])

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
      <div className='grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 xl:grid-cols-3'>
        {pagedModels.map((model) => (
          <ModelCard
            key={model.id ?? model.model_name}
            model={model}
            tokenUnit={tokenUnit}
            priceRate={props.priceRate}
            usdExchangeRate={props.usdExchangeRate}
            showRechargePrice={props.showRechargePrice}
            selectedGroup={props.selectedGroup}
            perf={perfMap.get(model.model_name || '')}
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
