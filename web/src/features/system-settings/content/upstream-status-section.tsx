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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
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
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

/**
 * Mirrors the server-side rule: an http(s) URL whose path ends in
 * /status/<slug>. The slug charset matches what the status page API accepts.
 */
const STATUS_PAGE_PATTERN = /^https?:\/\/[^\s/]+(?:\/[^\s/]+)*\/status\/[A-Za-z0-9_-]+\/?$/

const createUpstreamStatusSchema = (t: (key: string) => string) =>
  z.object({
    enabled: z.boolean(),
    url: z
      .string()
      .trim()
      .refine((value) => value === '' || STATUS_PAGE_PATTERN.test(value), {
        error: t(
          'Enter the full status page URL, for example https://status.example.com/status/default'
        ),
      }),
    name: z
      .string()
      .trim()
      .max(50, { error: t('Name must be less than 50 characters') }),
  })

type UpstreamStatusFormValues = z.infer<
  ReturnType<typeof createUpstreamStatusSchema>
>

type UpstreamStatusSectionProps = {
  enabled: boolean
  url: string
  name: string
}

/**
 * Mirrors an upstream provider's public status page into the console.
 *
 * The monitors show up in the console's uptime panel under the configured group
 * name, next to our own monitors, so a visitor can tell "our gateway is up" from
 * "the provider says its own service is up".
 */
export function UpstreamStatusSection(props: UpstreamStatusSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = createUpstreamStatusSchema(t)

  const form = useForm<UpstreamStatusFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      enabled: props.enabled,
      url: props.url,
      name: props.name,
    },
  })

  useEffect(() => {
    form.reset({ enabled: props.enabled, url: props.url, name: props.name })
  }, [props.enabled, props.url, props.name, form])

  const onSubmit = async (values: UpstreamStatusFormValues) => {
    if (values.enabled !== props.enabled) {
      await updateOption.mutateAsync({
        key: 'console_setting.upstream_status_enabled',
        value: values.enabled,
      })
    }
    if (values.url !== props.url) {
      await updateOption.mutateAsync({
        key: 'console_setting.upstream_status_url',
        value: values.url,
      })
    }
    if (values.name !== props.name) {
      await updateOption.mutateAsync({
        key: 'console_setting.upstream_status_name',
        value: values.name,
      })
    }
  }

  return (
    <SettingsSection title={t('Upstream Status')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save upstream status settings'
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex flex-row items-start justify-between gap-4 rounded-md border p-4'>
                <SettingsSwitchContent>
                  <FormLabel>{t('Show upstream status')}</FormLabel>
                  <FormDescription>
                    {t(
                      "Fetch the upstream provider's public status page and list its monitors in the console, next to your own."
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='url'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Upstream status page URL')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='https://status.example.com/status/default'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'The full status page URL, copied from the browser address bar. It must end in /status/<slug>.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Upstream group name')}</FormLabel>
                <FormControl>
                  <Input placeholder={t('Upstream Status')} {...field} />
                </FormControl>
                <FormDescription>
                  {t(
                    'The heading these monitors are listed under in the console.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
