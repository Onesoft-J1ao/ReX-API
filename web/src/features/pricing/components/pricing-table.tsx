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
import type { Row, PaginationState } from '@tanstack/react-table'
import { useMemo, useState, useCallback, useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePagination,
  DataTableRow,
  DataTableView,
  useDataTable,
} from '@/components/data-table'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'
import { getStatusCheck } from '@/features/status/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  DEFAULT_PRICING_TABLE_PAGE_SIZE,
  DEFAULT_TOKEN_UNIT,
  FILTER_ALL,
} from '../constants'
import type { GroupPricingOverrides } from '../lib/price'
import type { PricingModel, TokenUnit } from '../types'
import type { ModelPerfBadgeData } from './model-perf-badge'
import { usePricingColumns } from './pricing-columns'

export interface PricingTableProps {
  models: PricingModel[]
  isLoading?: boolean
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  /** Per-group pricing source overrides; absent when none are configured. */
  groupBillingSource?: Record<string, Record<string, string>>
  groupPricingOverrides?: GroupPricingOverrides
  onModelClick?: (modelName: string) => void
  pageSize?: number
}

export function PricingTable(props: PricingTableProps) {
  const { t } = useTranslation()
  const {
    models,
    isLoading = false,
    priceRate = 1,
    usdExchangeRate = 1,
    tokenUnit = DEFAULT_TOKEN_UNIT,
    showRechargePrice = false,
    selectedGroup,
    groupBillingSource,
    groupPricingOverrides,
    onModelClick,
  } = props

  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: props.pageSize ?? DEFAULT_PRICING_TABLE_PAGE_SIZE,
  })

  useEffect(() => {
    setPagination({
      pageIndex: 0,
      pageSize: props.pageSize ?? DEFAULT_PRICING_TABLE_PAGE_SIZE,
    })
  }, [props.pageSize])

  const perfQuery = useQuery({
    queryKey: ['perf-metrics-summary', 24],
    queryFn: async () => requireServerSuccess(await getPerfMetricsSummary(24)),
    staleTime: 60 * 1000,
    retry: false,
  })

  // Health is per (model, group), so the table needs the same group-keyed map
  // the card grid builds. Without it every row shows the passive rollup, which
  // has no group dimension and therefore cannot answer "how is this model doing
  // in the group the viewer picked".
  const healthQuery = useQuery({
    queryKey: ['status-check'],
    queryFn: async () => requireServerSuccess(await getStatusCheck()),
    staleTime: 60 * 1000,
    retry: false,
  })

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

  const healthByModelAndGroup = useMemo(() => {
    const map = new Map<string, ModelPerfBadgeData>()
    for (const group of healthQuery.data?.data?.groups ?? []) {
      for (const model of group.models ?? []) {
        // Keep every row that actually carries a reading (probe or passive
        // traffic); skip a served model with neither, so the row keeps the "—"
        // instead of an empty series.
        if (!model.model || model.has_data === false) continue
        map.set(`${group.group}|${model.model}`, {
          success_rate: model.availability,
          avg_latency_ms: model.latency_ms,
          // The probe does not measure throughput; the passive rollup below is
          // the only source that can fill it.
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

  // Mirrors ModelCardGrid.perfFor so both views answer the same question the
  // same way: a picked group shows that group's numbers when it has them, else
  // the model's own passive rollup, else nothing.
  const perfFor = useCallback(
    (modelName: string): ModelPerfBadgeData | undefined => {
      if (!modelName) return undefined
      if (selectedGroup && selectedGroup !== FILTER_ALL) {
        return (
          healthByModelAndGroup.get(`${selectedGroup}|${modelName}`) ??
          perfMap.get(modelName)
        )
      }
      for (const [key, value] of healthByModelAndGroup) {
        if (key.endsWith(`|${modelName}`)) return value
      }
      return perfMap.get(modelName)
    },
    [healthByModelAndGroup, perfMap, selectedGroup]
  )

  const columns = usePricingColumns({
    tokenUnit,
    priceRate,
    usdExchangeRate,
    showRechargePrice,
    selectedGroup,
    groupBillingSource,
    groupPricingOverrides,
    perfMap,
    perfFor,
  })

  const { table } = useDataTable({
    data: models,
    columns,
    pageCount: Math.ceil(models.length / pagination.pageSize),
    pagination,
    onPaginationChange: setPagination,
    manualPagination: false,
    withFilteredRowModel: false,
    withSortedRowModel: false,
    withFacetedRowModel: false,
  })

  const handleRowClick = useCallback(
    (model: PricingModel) => {
      onModelClick?.(model.model_name)
    },
    [onModelClick]
  )

  return (
    <div className='space-y-4'>
      <DataTableView
        table={table}
        isLoading={isLoading}
        emptyTitle={t('No Models Found')}
        emptyDescription={t('No models match your current filters.')}
        skeletonKeyPrefix='pricing-skeleton'
        applyHeaderSize
        getColumnClassName={(_columnId, kind) =>
          kind === 'header' ? 'text-muted-foreground font-medium' : undefined
        }
        renderRow={(row: Row<PricingModel>) => (
          <DataTableRow
            key={row.id}
            row={row}
            className='hover:bg-muted/30 cursor-pointer transition-colors'
            onClick={() => handleRowClick(row.original)}
          />
        )}
      />

      {!isLoading && models.length > 0 && <DataTablePagination table={table} />}
    </div>
  )
}
