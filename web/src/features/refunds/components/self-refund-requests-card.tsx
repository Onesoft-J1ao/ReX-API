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
import { RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { TitledCard } from '@/components/ui/titled-card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { createServerError } from '@/lib/server-error-message'

import { getSelfRefunds } from '../api'
import { ERROR_MESSAGES, REFUND_KINDS, REFUND_STATUSES } from '../constants'
import type { RefundRequest } from '../types'

const SELF_REFUND_PAGE_SIZE = 20

export function SelfRefundRequestsCard() {
  const { t } = useTranslation()

  const { data, isLoading } = useQuery({
    queryKey: ['self-refunds', SELF_REFUND_PAGE_SIZE],
    queryFn: async () => {
      const result = await getSelfRefunds({ p: 1, page_size: SELF_REFUND_PAGE_SIZE })
      if (!result.success) {
        throw createServerError(result, t(ERROR_MESSAGES.LOAD_FAILED))
      }
      return result.data?.items || []
    },
  })

  const requests = data || []

  const renderStatus = (request: RefundRequest) => {
    const config = REFUND_STATUSES[request.status]
    return (
      <StatusBadge
        label={config ? t(config.labelKey) : request.status}
        variant={config?.variant ?? 'neutral'}
        copyable={false}
      />
    )
  }

  const renderKind = (request: RefundRequest) => {
    const labelKey = REFUND_KINDS[request.kind]
    return labelKey ? t(labelKey) : request.kind
  }

  return (
    <TitledCard
      title={t('Refund Requests')}
      description={t('Track the refund requests created for your account')}
      icon={<RotateCcw className='h-4 w-4' />}
      iconTone='info'
      disableHoverEffect
      contentClassName='space-y-3'
    >
      {isLoading ? (
        <div className='space-y-2'>
          {['first', 'second'].map((key) => (
            <Skeleton key={key} className='h-14 w-full' />
          ))}
        </div>
      ) : requests.length === 0 ? (
        <p className='text-muted-foreground text-xs'>
          {t('You have no refund requests.')}
        </p>
      ) : (
        <ul className='max-h-72 space-y-2 overflow-y-auto pr-1'>
          {requests.map((request) => (
            <li
              key={request.id}
              className='bg-background rounded-md border p-3 text-xs'
            >
              <div className='flex items-center justify-between gap-2'>
                <span className='truncate font-medium'>
                  {renderKind(request)} · {t('Request')} #{request.id}
                </span>
                {renderStatus(request)}
              </div>
              <div className='text-muted-foreground mt-1.5'>
                {t('Refunded Quota')}: {formatQuota(request.quota)} ·{' '}
                {formatCurrencyFromUSD(request.amount)}
              </div>
              <div className='text-muted-foreground mt-0.5'>
                {t('Created')}: {formatTimestampToDate(request.created_at)}
              </div>
              {request.remark && (
                <div className='text-muted-foreground mt-0.5 truncate'>
                  {t('Remark')}: {request.remark}
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
    </TitledCard>
  )
}
