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
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import { REFUND_KINDS, REFUND_STATUSES } from '../constants'
import type { RefundRequest } from '../types'
import { RefundsRowActions } from './refunds-row-actions'

export function useRefundsColumns(): ColumnDef<RefundRequest>[] {
  const { t } = useTranslation()
  return [
    {
      accessorKey: 'id',
      header: t('ID'),
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <TableId value={row.getValue('id') as number} className='w-[60px]' />
      ),
      size: 80,
    },
    {
      accessorKey: 'user_id',
      header: t('User'),
      cell: ({ row }) => (
        <StatusBadge
          label={t('User {{id}}', { id: row.getValue('user_id') as number })}
          variant='neutral'
          copyable={false}
          className='-ml-1.5'
        />
      ),
      size: 140,
    },
    {
      accessorKey: 'kind',
      header: t('Type'),
      cell: ({ row }) => {
        const kind = row.getValue('kind') as string
        const labelKey = REFUND_KINDS[kind]
        return (
          <StatusBadge
            label={labelKey ? t(labelKey) : kind}
            variant='neutral'
            copyable={false}
            className='-ml-1.5'
          />
        )
      },
      size: 140,
    },
    {
      accessorKey: 'status',
      header: t('Status'),
      meta: { mobileBadge: true },
      cell: ({ row }) => {
        const status = row.getValue('status') as string
        const statusConfig = REFUND_STATUSES[status]

        if (!statusConfig) {
          return (
            <StatusBadge
              label={status}
              variant='neutral'
              copyable={false}
              className='-ml-1.5'
            />
          )
        }

        return (
          <StatusBadge
            label={t(statusConfig.labelKey)}
            variant={statusConfig.variant}
            copyable={false}
            className='-ml-1.5'
          />
        )
      },
      size: 120,
    },
    {
      accessorKey: 'quota',
      header: t('Refunded Quota'),
      cell: ({ row }) => (
        <StatusBadge
          label={formatQuota(row.getValue('quota') as number)}
          variant='neutral'
          copyable={false}
          className='-ml-1.5'
        />
      ),
      size: 140,
    },
    {
      accessorKey: 'amount',
      header: t('Amount'),
      cell: ({ row }) => (
        <span className='font-mono text-sm'>
          {formatCurrencyFromUSD(row.getValue('amount') as number)}
        </span>
      ),
      size: 140,
    },
    {
      accessorKey: 'created_at',
      header: t('Created'),
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <div className='min-w-[160px] font-mono text-sm'>
          {formatTimestampToDate(row.getValue('created_at'))}
        </div>
      ),
      size: 180,
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      cell: ({ row }) => <RefundsRowActions row={row} />,
      meta: { pinned: 'right' as const },
    },
  ]
}
