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
import { useEffect, useMemo, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

const numericString = z.string().refine((value) => {
  const trimmed = value.trim()
  if (!trimmed) return true
  return !Number.isNaN(Number(trimmed)) && Number(trimmed) >= 0
}, 'Enter a non-negative number or leave empty')

const perfMetricsBucketLabel = {
  minute: '1 minute',
  '5min': '5 minutes',
  hour: '1 hour',
} as const

const monitoringSchema = z.object({
  QuotaRemindEnabled: z.boolean(),
  QuotaRemindThreshold: numericString,
  perf_metrics_setting: z.object({
    enabled: z.boolean(),
    exclude_upstream_400_errors: z.boolean(),
    flush_interval: z.coerce.number().min(1),
    bucket_time: z.enum(['minute', '5min', 'hour']),
    retention_days: z.coerce.number().min(0),
    upstream_enabled: z.boolean(),
    upstream_summary_url: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^https?:\/\//.test(trimmed)
    }, 'Provide a valid URL starting with http:// or https://'),
    upstream_timeout_seconds: z.coerce.number().min(1),
  }),
  status_check_setting: z.object({
    probe_free_models_only: z.boolean(),
  }),
  model_catalog_setting: z.object({
    enabled: z.boolean(),
    source_url: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^https?:\/\//.test(trimmed)
    }, 'Provide a valid URL starting with http:// or https://'),
    timeout_seconds: z.coerce.number().min(1),
    cache_minutes: z.coerce.number().min(1),
  }),
})

type MonitoringFormInput = z.input<typeof monitoringSchema>
type MonitoringFormValues = z.output<typeof monitoringSchema>

type FlatMonitoringDefaults = {
  QuotaRemindEnabled: boolean
  QuotaRemindThreshold: string
  'perf_metrics_setting.enabled': boolean
  'perf_metrics_setting.exclude_upstream_400_errors': boolean
  'perf_metrics_setting.flush_interval': number
  'perf_metrics_setting.bucket_time': 'minute' | '5min' | 'hour'
  'perf_metrics_setting.retention_days': number
  'perf_metrics_setting.upstream_enabled': boolean
  'perf_metrics_setting.upstream_summary_url': string
  'perf_metrics_setting.upstream_timeout_seconds': number
  'status_check_setting.probe_free_models_only': boolean
  'model_catalog_setting.enabled': boolean
  'model_catalog_setting.source_url': string
  'model_catalog_setting.timeout_seconds': number
  'model_catalog_setting.cache_minutes': number
}

type MonitoringSettingsSectionProps = {
  defaultValues: FlatMonitoringDefaults
}

const buildFormDefaults = (
  defaults: MonitoringSettingsSectionProps['defaultValues']
): MonitoringFormInput => ({
  QuotaRemindEnabled: defaults.QuotaRemindEnabled,
  QuotaRemindThreshold: defaults.QuotaRemindThreshold ?? '',
  perf_metrics_setting: {
    enabled: defaults['perf_metrics_setting.enabled'],
    exclude_upstream_400_errors:
      defaults['perf_metrics_setting.exclude_upstream_400_errors'],
    flush_interval: defaults['perf_metrics_setting.flush_interval'],
    bucket_time: defaults['perf_metrics_setting.bucket_time'],
    retention_days: defaults['perf_metrics_setting.retention_days'],
    upstream_enabled: defaults['perf_metrics_setting.upstream_enabled'],
    upstream_summary_url:
      defaults['perf_metrics_setting.upstream_summary_url'] ?? '',
    upstream_timeout_seconds:
      defaults['perf_metrics_setting.upstream_timeout_seconds'],
  },
  status_check_setting: {
    probe_free_models_only:
      defaults['status_check_setting.probe_free_models_only'],
  },
  model_catalog_setting: {
    enabled: defaults['model_catalog_setting.enabled'],
    source_url: defaults['model_catalog_setting.source_url'] ?? '',
    timeout_seconds: defaults['model_catalog_setting.timeout_seconds'],
    cache_minutes: defaults['model_catalog_setting.cache_minutes'],
  },
})

const normalizeDefaults = (
  defaults: MonitoringSettingsSectionProps['defaultValues']
): FlatMonitoringDefaults => ({
  QuotaRemindEnabled: defaults.QuotaRemindEnabled,
  QuotaRemindThreshold: (defaults.QuotaRemindThreshold ?? '').trim(),
  'perf_metrics_setting.enabled': defaults['perf_metrics_setting.enabled'],
  'perf_metrics_setting.exclude_upstream_400_errors':
    defaults['perf_metrics_setting.exclude_upstream_400_errors'],
  'perf_metrics_setting.flush_interval':
    defaults['perf_metrics_setting.flush_interval'],
  'perf_metrics_setting.bucket_time':
    defaults['perf_metrics_setting.bucket_time'],
  'perf_metrics_setting.retention_days':
    defaults['perf_metrics_setting.retention_days'],
  'perf_metrics_setting.upstream_enabled':
    defaults['perf_metrics_setting.upstream_enabled'],
  'perf_metrics_setting.upstream_summary_url': (
    defaults['perf_metrics_setting.upstream_summary_url'] ?? ''
  ).trim(),
  'perf_metrics_setting.upstream_timeout_seconds':
    defaults['perf_metrics_setting.upstream_timeout_seconds'],
  'status_check_setting.probe_free_models_only':
    defaults['status_check_setting.probe_free_models_only'],
  'model_catalog_setting.enabled': defaults['model_catalog_setting.enabled'],
  'model_catalog_setting.source_url': (
    defaults['model_catalog_setting.source_url'] ?? ''
  ).trim(),
  'model_catalog_setting.timeout_seconds':
    defaults['model_catalog_setting.timeout_seconds'],
  'model_catalog_setting.cache_minutes':
    defaults['model_catalog_setting.cache_minutes'],
})

const normalizeFormValues = (
  values: MonitoringFormValues
): FlatMonitoringDefaults => ({
  QuotaRemindEnabled: values.QuotaRemindEnabled,
  QuotaRemindThreshold: values.QuotaRemindThreshold.trim(),
  'perf_metrics_setting.enabled': values.perf_metrics_setting.enabled,
  'perf_metrics_setting.exclude_upstream_400_errors':
    values.perf_metrics_setting.exclude_upstream_400_errors,
  'perf_metrics_setting.flush_interval':
    values.perf_metrics_setting.flush_interval,
  'perf_metrics_setting.bucket_time': values.perf_metrics_setting.bucket_time,
  'perf_metrics_setting.retention_days':
    values.perf_metrics_setting.retention_days,
  'perf_metrics_setting.upstream_enabled':
    values.perf_metrics_setting.upstream_enabled,
  'perf_metrics_setting.upstream_summary_url':
    values.perf_metrics_setting.upstream_summary_url.trim(),
  'perf_metrics_setting.upstream_timeout_seconds':
    values.perf_metrics_setting.upstream_timeout_seconds,
  'status_check_setting.probe_free_models_only':
    values.status_check_setting.probe_free_models_only,
  'model_catalog_setting.enabled': values.model_catalog_setting.enabled,
  'model_catalog_setting.source_url':
    values.model_catalog_setting.source_url.trim(),
  'model_catalog_setting.timeout_seconds':
    values.model_catalog_setting.timeout_seconds,
  'model_catalog_setting.cache_minutes':
    values.model_catalog_setting.cache_minutes,
})

export function MonitoringSettingsSection({
  defaultValues,
}: MonitoringSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef<FlatMonitoringDefaults>(
    normalizeDefaults(defaultValues)
  )
  const baselineSerializedRef = useRef<string>(
    JSON.stringify(normalizeDefaults(defaultValues))
  )

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<MonitoringFormInput, unknown, MonitoringFormValues>({
    resolver: zodResolver(monitoringSchema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  useEffect(() => {
    const normalized = normalizeDefaults(defaultValues)
    const serialized = JSON.stringify(normalized)
    if (serialized === baselineSerializedRef.current) return
    baselineRef.current = normalized
    baselineSerializedRef.current = serialized
  }, [defaultValues])

  const perfMetricsEnabled = form.watch('perf_metrics_setting.enabled')
  const upstreamEnabled = form.watch('perf_metrics_setting.upstream_enabled')

  const onSubmit = async (values: MonitoringFormValues) => {
    const normalized = normalizeFormValues(values)
    const updates = (
      Object.keys(normalized) as Array<keyof FlatMonitoringDefaults>
    ).filter((key) => normalized[key] !== baselineRef.current[key])

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      await updateOption.mutateAsync({
        key,
        value: normalized[key],
      })
    }

    baselineRef.current = normalized
    baselineSerializedRef.current = JSON.stringify(normalized)
  }

  return (
    <SettingsSection title={t('Monitoring & Alerts')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='QuotaRemindEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable balance warnings')}</FormLabel>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t('Send balance and subscription quota warning emails')}
                  </p>
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
            name='QuotaRemindThreshold'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Quota reminder (tokens)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    step={1}
                    value={field.value}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Send email alerts when a user falls below this quota')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div>
            <h4 className='font-medium'>{t('Model performance metrics')}</h4>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Collect relay latency and success-rate metrics for the model square.'
              )}
            </p>
          </div>

          <FormField
            control={form.control}
            name='perf_metrics_setting.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable model performance metrics')}</FormLabel>
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
            name='perf_metrics_setting.exclude_upstream_400_errors'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Exclude upstream HTTP 400 errors')}</FormLabel>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t(
                      'Do not include upstream HTTP 400 responses in request success-rate metrics.'
                    )}
                  </p>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={!perfMetricsEnabled}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='perf_metrics_setting.flush_interval'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Flush interval (minutes)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!perfMetricsEnabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='perf_metrics_setting.bucket_time'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Aggregation bucket')}</FormLabel>
                  <Select
                    items={[
                      { value: 'minute', label: t('1 minute') },
                      { value: '5min', label: t('5 minutes') },
                      { value: 'hour', label: t('1 hour') },
                    ]}
                    value={field.value}
                    onValueChange={field.onChange}
                    disabled={!perfMetricsEnabled}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue>
                          {t(perfMetricsBucketLabel[field.value])}
                        </SelectValue>
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='minute'>{t('1 minute')}</SelectItem>
                        <SelectItem value='5min'>{t('5 minutes')}</SelectItem>
                        <SelectItem value='hour'>{t('1 hour')}</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='perf_metrics_setting.retention_days'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Retention days')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!perfMetricsEnabled}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('0 means data is kept permanently')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div>
            <h4 className='font-medium'>{t('Mirrored upstream health')}</h4>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Reuse another instance’s published health data. It covers models we do not sample ourselves, including pay-per-request models we avoid probing.'
              )}
            </p>
          </div>

          <FormField
            control={form.control}
            name='perf_metrics_setting.upstream_enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Mirror upstream health data')}</FormLabel>
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

          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='perf_metrics_setting.upstream_summary_url'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Upstream summary URL')}</FormLabel>
                  <FormControl>
                    <Input
                      type='url'
                      inputMode='url'
                      placeholder={t(
                        'https://example.com/api/perf-metrics/summary'
                      )}
                      autoComplete='off'
                      disabled={!upstreamEnabled}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'The other instance’s perf-metrics summary endpoint.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='perf_metrics_setting.upstream_timeout_seconds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Upstream timeout (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!upstreamEnabled}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'A failed fetch keeps showing the last successful data.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div>
            <h4 className='font-medium'>{t('Active probing')}</h4>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Probing sends a real request upstream, so a pay-per-request model costs money on every cycle.'
              )}
            </p>
          </div>

          <FormField
            control={form.control}
            name='status_check_setting.probe_free_models_only'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Only probe free models')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Skip paid groups entirely. Their health comes from the mirrored upstream instead.'
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

          <div>
            <h4 className='font-medium'>{t('Model catalog')}</h4>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Fill the model detail view with context window, modalities and capabilities. The gateway stores billing data and has no column for these, so they are read from an external catalog and merged into the pricing response.'
              )}
            </p>
          </div>

          <FormField
            control={form.control}
            name='model_catalog_setting.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable model catalog')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Show context window, modalities and capabilities on the model detail view.'
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
            name='model_catalog_setting.source_url'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Catalog source URL')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='https://models.dev/api.json'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Point this at a mirror when the public catalog is unreachable from your region.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='model_catalog_setting.timeout_seconds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Catalog fetch timeout (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('The catalog document is a few megabytes.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='model_catalog_setting.cache_minutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Catalog cache (minutes)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'The catalog changes daily, so a long cache costs nothing.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
