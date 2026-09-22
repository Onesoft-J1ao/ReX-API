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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  StaticDataTable,
  staticDataTableClassNames as tableStyles,
} from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { handleServerError } from '@/lib/handle-server-error'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getPriceSentinelReport, runPriceSentinelNow } from '../../price-sentinel/api'
import type { PriceChange } from '../../price-sentinel/types'
import { SettingsSection } from '../components/settings-section'

export function PriceSentinelSection() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()

  const reportQuery = useQuery({
    queryKey: ['price-sentinel-report'],
    queryFn: getPriceSentinelReport,
    staleTime: 60 * 1000,
  })

  const runNow = useMutation({
    mutationFn: runPriceSentinelNow,
    onSuccess: () => {
      toast.success(t('Price check finished'))
      void queryClient.invalidateQueries({ queryKey: ['price-sentinel-report'] })
    },
    onError: (error: Error) => handleServerError(error, t('Price check failed')),
  })

  const report = reportQuery.data?.report ?? null
  const changes = reportQuery.data?.changes ?? []

  return (
    <SettingsSection title={t('Upstream price sentinel')}>
      <div className='space-y-6'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <p className='text-muted-foreground max-w-2xl text-sm'>
            {t(
              'Compares our prices with the mirrored upstream every hour and emails a digest of what differs. It only reports: a price changes only when you apply it from the emailed link.'
            )}
          </p>
          <Button
            variant='outline'
            disabled={runNow.isPending}
            onClick={() => runNow.mutate()}
          >
            <RefreshCw
              className={runNow.isPending ? 'animate-spin' : undefined}
            />
            {runNow.isPending ? t('Checking...') : t('Check now')}
          </Button>
        </div>

        {reportQuery.isLoading && <LoadingState />}

        {!reportQuery.isLoading && !report && (
          <EmptyState
            title={t('No check has run yet')}
            description={t(
              'The sentinel runs hourly. Use “Check now” to run one immediately.'
            )}
            bordered
          />
        )}

        {report && (
          <>
            {report.fetch_error !== '' && (
              <div className='border-destructive/40 bg-destructive/5 text-destructive flex items-start gap-2 rounded-xl border p-3 text-sm'>
                <AlertTriangle className='mt-0.5 size-4 shrink-0' />
                <span>{report.fetch_error}</span>
              </div>
            )}

            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
              <div className='rounded-xl border p-4'>
                <div className='text-muted-foreground text-sm'>
                  {t('Last checked')}
                </div>
                <div className='font-semibold'>
                  {new Date(report.checked_at * 1000).toLocaleString(locale)}
                </div>
              </div>
              <div className='rounded-xl border p-4'>
                <div className='text-muted-foreground text-sm'>
                  {t('Models compared')}
                </div>
                <div className='font-mono font-semibold tabular-nums'>
                  {formatNumber(report.compared, locale)}
                </div>
              </div>
              <div className='rounded-xl border p-4'>
                <div className='text-muted-foreground text-sm'>
                  {t('We charge more')}
                </div>
                <div className='font-mono font-semibold tabular-nums'>
                  {formatNumber(report.higher_price, locale)}
                </div>
              </div>
              <div className='rounded-xl border p-4'>
                <div className='text-muted-foreground text-sm'>
                  {t('We charge less')}
                </div>
                <div className='font-mono font-semibold tabular-nums'>
                  {formatNumber(report.lower_price, locale)}
                </div>
              </div>
            </div>

            {changes.length === 0 ? (
              <EmptyState
                title={t('Our prices match the upstream')}
                description={t(
                  'Nothing differed beyond the reporting threshold in the last check.'
                )}
                bordered
              />
            ) : (
              <StaticDataTable
                className='rounded-xl'
                tableClassName='text-sm'
                headerRowClassName={tableStyles.compactHeaderRow}
                data={changes}
                getRowKey={(change) => `${change.model_name}|${change.field}`}
                columns={[
                  {
                    id: 'model',
                    header: t('Model'),
                    className: tableStyles.compactHeaderCell,
                    cellClassName: tableStyles.compactCell,
                    cell: (change: PriceChange) => (
                      <div className='flex flex-col gap-1'>
                        <span className='break-all'>{change.model_name}</span>
                        {change.groups && change.groups.length > 0 && (
                          <span className='flex flex-wrap gap-1'>
                            {change.groups.map((group) => (
                              <Badge key={group} variant='outline'>
                                {group}
                              </Badge>
                            ))}
                          </span>
                        )}
                      </div>
                    ),
                  },
                  {
                    id: 'field',
                    header: t('Field'),
                    className: tableStyles.compactHeaderCell,
                    cellClassName: cn(tableStyles.compactCell, 'font-mono text-xs'),
                    cell: (change: PriceChange) => change.field,
                  },
                  {
                    id: 'ours',
                    header: t('Ours'),
                    className: tableStyles.compactHeaderCellRight,
                    cellClassName: tableStyles.compactNumericCell,
                    cell: (change: PriceChange) =>
                      change.ours == null
                        ? t('Not set')
                        : formatNumber(change.ours, locale),
                  },
                  {
                    id: 'theirs',
                    header: t('Upstream'),
                    className: tableStyles.compactHeaderCellRight,
                    cellClassName: tableStyles.compactNumericCell,
                    cell: (change: PriceChange) =>
                      change.theirs == null
                        ? t('Not set')
                        : formatNumber(change.theirs, locale),
                  },
                  {
                    id: 'percent',
                    header: t('Difference'),
                    className: tableStyles.compactHeaderCellRight,
                    cellClassName: tableStyles.compactNumericCell,
                    cell: (change: PriceChange) =>
                      change.percent === 0
                        ? '—'
                        : `${change.percent > 0 ? '+' : ''}${formatNumber(change.percent, locale)}%`,
                  },
                ]}
              />
            )}
          </>
        )}
      </div>
    </SettingsSection>
  )
}
