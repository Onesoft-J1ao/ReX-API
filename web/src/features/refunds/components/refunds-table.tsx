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
import { getRouteApi } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { createServerError } from '@/lib/server-error-message'

import { getRefunds } from '../api'
import { ERROR_MESSAGES, getRefundStatusOptions } from '../constants'
import { useRefundsColumns } from './refunds-columns'
import { useRefunds } from './refunds-provider'

const route = getRouteApi('/_authenticated/refunds/')

export function RefundsTable() {
  const { t } = useTranslation()
  const columns = useRefundsColumns()
  const { refreshTrigger } = useRefunds()
  const isMobile = useMediaQuery('(max-width: 640px)')

  const {
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    columnFilters: [{ columnId: 'status', searchKey: 'status', type: 'array' }],
  })
  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | string[]
      | undefined) ?? []
  const statusFilterValue = statusFilter[0] ?? ''

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'refunds',
      pagination.pageIndex + 1,
      pagination.pageSize,
      statusFilterValue,
      refreshTrigger,
    ],
    queryFn: async () => {
      const result = await getRefunds({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        status: statusFilterValue,
      })

      if (!result.success) {
        throw createServerError(result, t(ERROR_MESSAGES.LOAD_FAILED))
      }

      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const refunds = data?.items || []

  const { table } = useDataTable({
    data: refunds,
    columns,
    getRowId: (row) => String(row.id),
    columnFilters,
    pagination,
    onPaginationChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total || 0,
    ensurePageInRange,
  })

  const refundStatusOptions = useMemo(() => getRefundStatusOptions(t), [t])

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Refund Requests Found')}
      emptyDescription={t(
        'No refund requests have been created yet. Refund requests appear here once an administrator creates one.'
      )}
      skeletonKeyPrefix='refunds-skeleton'
      applyHeaderSize
      toolbarProps={{
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: refundStatusOptions,
            singleSelect: true,
          },
        ],
      }}
      mobileProps={{ getRowKey: (row) => String(row.original.id) }}
    />
  )
}
