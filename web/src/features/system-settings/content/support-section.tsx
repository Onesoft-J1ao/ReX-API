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

const createSupportSchema = (t: (key: string) => string) =>
  z.object({
    enabled: z.boolean(),
    url: z
      .string()
      .trim()
      .refine((value) => value === '' || /^https?:\/\/\S+$/.test(value), {
        error: t('Enter the full QQ group join link'),
      }),
  })

type SupportFormValues = z.infer<ReturnType<typeof createSupportSchema>>

type SupportSectionProps = {
  enabled: boolean
  url: string
}

/**
 * The QQ group join link shown on the user dashboard.
 *
 * The value is the link the group's own "join" widget points at — it carries
 * the group key and an auth key. Operators copy it from the widget rather than
 * entering a group number, because a bare group number cannot be turned into a
 * working join link.
 */
export function SupportSection(props: SupportSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = createSupportSchema(t)

  const form = useForm<SupportFormValues>({
    resolver: zodResolver(schema),
    defaultValues: { enabled: props.enabled, url: props.url },
  })

  useEffect(() => {
    form.reset({ enabled: props.enabled, url: props.url })
  }, [props.enabled, props.url, form])

  const onSubmit = async (values: SupportFormValues) => {
    if (values.enabled !== props.enabled) {
      await updateOption.mutateAsync({
        key: 'console_setting.support_qq_enabled',
        value: values.enabled,
      })
    }
    if (values.url !== props.url) {
      await updateOption.mutateAsync({
        key: 'console_setting.support_qq_url',
        value: values.url,
      })
    }
  }

  return (
    <SettingsSection title={t('Support group')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save support group settings'
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex flex-row items-start justify-between gap-4 rounded-md border p-4'>
                <SettingsSwitchContent>
                  <FormLabel>{t('Show support group')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Show a card on the user dashboard that opens your QQ group join link.'
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
                <FormLabel>{t('QQ group join link')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='https://qm.qq.com/cgi-bin/qm/qr?k=...'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Paste the link the QQ group join widget points at. Leave empty to hide the card.'
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
