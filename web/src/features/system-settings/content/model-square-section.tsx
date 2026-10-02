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
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const modelSquareSchema = z.object({
  hideWithoutEnabledChannel: z.boolean(),
})

type ModelSquareFormValues = z.infer<typeof modelSquareSchema>

type ModelSquareSectionProps = {
  hideWithoutEnabledChannel: boolean
}

/**
 * Model square display settings.
 *
 * The one switch here decides whether a group whose channels are all disabled
 * still appears in the group picker and the model list.
 */
export function ModelSquareSection(props: ModelSquareSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const form = useForm<ModelSquareFormValues>({
    resolver: zodResolver(modelSquareSchema),
    defaultValues: { hideWithoutEnabledChannel: props.hideWithoutEnabledChannel },
  })

  useEffect(() => {
    form.reset({ hideWithoutEnabledChannel: props.hideWithoutEnabledChannel })
  }, [props.hideWithoutEnabledChannel, form])

  const onSubmit = async (values: ModelSquareFormValues) => {
    if (values.hideWithoutEnabledChannel === props.hideWithoutEnabledChannel) {
      return
    }
    await updateOption.mutateAsync({
      key: 'console_setting.hide_models_without_enabled_channel',
      value: values.hideWithoutEnabledChannel ? 'true' : 'false',
    })
  }

  return (
    <SettingsSection title={t('Model Square')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save model square settings'
          />
          <FormField
            control={form.control}
            name='hideWithoutEnabledChannel'
            render={({ field }) => (
              <FormItem className='flex flex-row items-start justify-between gap-4 rounded-md border p-4'>
                <SettingsSwitchContent>
                  <FormLabel>{t('Hide groups with no usable channel')}</FormLabel>
                  <FormDescription>
                    {t(
                      'A group whose channels are all disabled is removed from the group picker and the model list, so visitors are not offered a group that cannot serve anything.'
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
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
