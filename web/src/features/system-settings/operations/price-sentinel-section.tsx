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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import type { Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import {
  StaticDataTable,
  staticDataTableClassNames as tableStyles,
} from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toIntlLocale } from '@/i18n/languages'
import { handleServerError } from '@/lib/handle-server-error'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getPriceSentinelReport, runPriceSentinelNow } from '../../price-sentinel/api'
import type { PriceChange } from '../../price-sentinel/types'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

// Field names are the option keys, so a save writes exactly the keys the form
// changed. The time and email inputs carry their own type constraints, which
// keeps the schema free of messages that would need translating.
const priceSentinelSettingsSchema = z.object({
  'price_sentinel_setting.enabled': z.boolean(),
  'price_sentinel_setting.interval_minutes': z.coerce
    .number()
    .int()
    .min(1)
    .max(1440),
  'price_sentinel_setting.send_time': z.string(),
  'price_sentinel_setting.report_email': z.string(),
  // A fraction, not a percentage, because that is what the check and the sync
  // both multiply by. The server clamps to 10 (1000%).
  'price_sentinel_setting.margin_percent': z.coerce.number().min(0).max(10),
})

export type PriceSentinelSettingsValues = z.infer<
  typeof priceSentinelSettingsSchema
>

type PriceSentinelSectionProps = {
  defaultValues: PriceSentinelSettingsValues
}

export function PriceSentinelSection(props: PriceSentinelSectionProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<PriceSentinelSettingsValues>({
      resolver: zodResolver(priceSentinelSettingsSchema) as Resolver<
        PriceSentinelSettingsValues,
        unknown,
        PriceSentinelSettingsValues
      >,
      defaultValues: props.defaultValues,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          await updateOption.mutateAsync({
            key,
            value: value as string | number | boolean,
          })
        }
      },
    })
  const intervalMinutes = form.watch('price_sentinel_setting.interval_minutes')

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
      <FormNavigationGuard when={isDirty} />
      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={updateOption.isPending || isSubmitting}
          />
          <FormDirtyIndicator isDirty={isDirty} />
          <FormField
            control={form.control}
            name='price_sentinel_setting.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable the price sentinel')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Compare the upstream prices on the schedule below and email the digest.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
          <FormField
            control={form.control}
            name='price_sentinel_setting.interval_minutes'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Check interval (minutes)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    max={1440}
                    step={1}
                    value={field.value}
                    onChange={(event) => field.onChange(event.target.value)}
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'How often the check runs. 60 is hourly, 300 is every five hours.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='price_sentinel_setting.margin_percent'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Markup factor')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    max={10}
                    step={0.05}
                    value={field.value}
                    onChange={(event) => field.onChange(event.target.value)}
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'A price sync writes upstream times one plus this: 0.2 is 20% above the upstream price. Both the sentinel and the upstream price sync use it, so they cannot drift apart.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='price_sentinel_setting.send_time'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Send time')}</FormLabel>
                <FormControl>
                  <Input
                    type='time'
                    value={field.value}
                    onChange={(event) => field.onChange(event.target.value)}
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Optional. Anchors the schedule to this time of day: the digest goes out then and every interval after it. Leave it empty to count from the previous run instead.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='price_sentinel_setting.report_email'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Digest recipient')}</FormLabel>
                <FormControl>
                  <Input
                    type='email'
                    placeholder={t('Defaults to the root administrator')}
                    value={field.value}
                    onChange={(event) => field.onChange(event.target.value)}
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Leave empty to send the digest to the root administrator.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>

      <div className='space-y-6'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <p className='text-muted-foreground max-w-2xl text-sm'>
            {t(
              'Compares our prices with the mirrored upstream on the schedule above and emails a digest of what differs. It only reports: a price changes only when you apply it from the emailed link.'
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
              'The sentinel runs every {{minutes}} minutes. Use “Check now” to run one immediately.',
              { minutes: intervalMinutes }
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
