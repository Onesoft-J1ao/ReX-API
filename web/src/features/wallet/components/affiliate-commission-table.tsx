/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import { formatQuota } from '@/lib/format'

type AffCommissionItem = {
  id: number
  invitee_id: number
  money: number
  quota: number
  status: string
  available_at: number
  credited_at: number
}

type AffCommissionData = {
  pending: number
  credited: number
  cancelled: number
  items: AffCommissionItem[]
}

const EMPTY: AffCommissionData = {
  pending: 0,
  credited: 0,
  cancelled: 0,
  items: [],
}

function statusLabel(status: string, t: (key: string) => string): string {
  if (status === 'credited') return t('Credited')
  if (status === 'pending') return t('Pending')
  return t('Cancelled')
}

function formatTime(ts: number): string {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleString()
}

export function AffiliateCommissionTable() {
  const { t } = useTranslation()

  const { data, isLoading } = useQuery({
    queryKey: ['aff-commissions'],
    queryFn: async () => {
      const res = await api.get('/api/user/aff/commissions')
      const payload = res.data?.data
      return (payload ?? EMPTY) as AffCommissionData
    },
    staleTime: 60 * 1000,
  })

  const rows = data?.items ?? []

  return (
    <Card data-card-hover='false' className='py-0'>
      <CardHeader className='gap-2 border-b p-3 sm:p-4'>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t(
            'Invitees earn a 7.5% rebate for each of their first 10 fiat top-ups above 15 CNY. The rebate is credited to your balance 24 hours later. Rewards are for platform usage only and cannot be withdrawn. Gifted quota, redemption codes, subscriptions, crypto payments and manual top-ups do not count.'
          )}
        </p>
        <div className='grid grid-cols-3 gap-2 pt-1 text-center'>
          {[
            [t('Pending'), formatQuota(data?.pending ?? 0)],
            [t('Credited'), formatQuota(data?.credited ?? 0)],
            [t('Cancelled'), formatQuota(data?.cancelled ?? 0)],
          ].map(([label, value]) => (
            <div key={label} className='bg-muted/30 rounded-md px-2 py-1.5'>
              <div className='text-muted-foreground text-[11px] uppercase'>
                {label}
              </div>
              <div className='mt-0.5 font-mono text-sm font-semibold tabular-nums'>
                {value}
              </div>
            </div>
          ))}
        </div>
      </CardHeader>
      <CardContent className='p-0'>
        {isLoading && (
          <div className='space-y-2 p-3'>
            <Skeleton className='h-6 w-full' />
            <Skeleton className='h-6 w-full' />
          </div>
        )}
        {!isLoading && (
          <div className='overflow-x-auto'>
            <table className='w-full text-xs'>
              <thead className='text-muted-foreground border-b'>
                <tr>
                  <th className='px-3 py-2 text-left font-medium'>
                    {t('Invitee ID')}
                  </th>
                  <th className='px-3 py-2 text-right font-medium'>
                    {t('Paid (CNY)')}
                  </th>
                  <th className='px-3 py-2 text-right font-medium'>
                    {t('Reward')}
                  </th>
                  <th className='px-3 py-2 text-left font-medium'>
                    {t('Status')}
                  </th>
                  <th className='px-3 py-2 text-left font-medium'>
                    {t('Estimated credit time')}
                  </th>
                  <th className='px-3 py-2 text-left font-medium'>
                    {t('Credited at')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.length === 0 && (
                  <tr>
                    <td
                      colSpan={6}
                      className='text-muted-foreground px-3 py-6 text-center'
                    >
                      {t('No referral rebate records yet')}
                    </td>
                  </tr>
                )}
                {rows.map((row) => (
                  <tr key={row.id} className='border-b last:border-0'>
                    <td className='px-3 py-2 font-mono'>{row.invitee_id}</td>
                    <td className='px-3 py-2 text-right tabular-nums'>
                      {row.money.toFixed(2)}
                    </td>
                    <td className='px-3 py-2 text-right tabular-nums'>
                      {formatQuota(row.quota)}
                    </td>
                    <td className='px-3 py-2'>{statusLabel(row.status, t)}</td>
                    <td className='px-3 py-2 whitespace-nowrap'>
                      {formatTime(row.available_at)}
                    </td>
                    <td className='px-3 py-2 whitespace-nowrap'>
                      {formatTime(row.credited_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <div className='text-muted-foreground border-t px-3 py-2 text-xs'>
          {t('Total')}: {formatQuota(
            (data?.pending ?? 0) + (data?.credited ?? 0) + (data?.cancelled ?? 0)
          )}
        </div>
      </CardContent>
    </Card>
  )
}
