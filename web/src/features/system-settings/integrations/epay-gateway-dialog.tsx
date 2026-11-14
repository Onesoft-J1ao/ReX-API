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

import { Dialog } from '@/components/dialog'
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

import type { EpayGatewayData } from './epay-gateways'
import { PaymentMethodsVisualEditor } from './payment-methods-visual-editor'

const EPAY_GATEWAY_FORM_ID = 'epay-gateway-form'

type EpayGatewayDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (gateway: EpayGatewayData) => void
  editData: EpayGatewayData | null
  existingIds: string[]
}

export function EpayGatewayDialog(props: EpayGatewayDialogProps) {
  const { t } = useTranslation()
  const schema = z.object({
    id: z
      .string()
      .trim()
      .min(1, t('Gateway ID is required'))
      .max(100, t('Gateway ID must not exceed 100 characters'))
      .refine(
        (value) =>
          value === props.editData?.id || !props.existingIds.includes(value),
        t('Gateway ID must be unique')
      ),
    name: z.string().trim().min(1, t('Gateway name is required')),
    address: z
      .url(t('Enter a valid Epay gateway URL'))
      .refine(
        (value) => value.startsWith('http://') || value.startsWith('https://'),
        t('Enter a valid Epay gateway URL')
      ),
    merchant_id: z.string().trim().min(1, t('Merchant ID is required')),
    key: z.string().trim().min(1, t('Secret key is required')),
    enabled: z.boolean(),
    pay_methods: z.string(),
    schedule_enabled: z.boolean(),
    schedule_start: z
      .string()
      .trim()
      .refine(
        (value) => value === '' || /^([01]\d|2[0-3]):[0-5]\d$/.test(value),
        t('Use HH:MM (24-hour clock)')
      ),
    schedule_end: z
      .string()
      .trim()
      .refine(
        (value) => value === '' || /^([01]\d|2[0-3]):[0-5]\d$/.test(value),
        t('Use HH:MM (24-hour clock)')
      ),
  })
  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      id: '',
      name: '',
      address: '',
      merchant_id: '',
      key: '',
      enabled: true,
      pay_methods: '[]',
      schedule_enabled: false,
      schedule_start: '',
      schedule_end: '',
    },
  })

  useEffect(() => {
    const gateway = props.editData
    form.reset(
      gateway
        ? {
            ...gateway,
            enabled: gateway.enabled ?? true,
            schedule_enabled: gateway.schedule_enabled ?? false,
            schedule_start: gateway.schedule_start ?? '',
            schedule_end: gateway.schedule_end ?? '',
            pay_methods: JSON.stringify(gateway.pay_methods, null, 2),
          }
        : {
            id: '',
            name: '',
            address: '',
            merchant_id: '',
            key: '',
            enabled: true,
            pay_methods: '[]',
            schedule_enabled: false,
            schedule_start: '',
            schedule_end: '',
          }
    )
  }, [form, props.editData, props.open])

  const handleSubmit = (values: FormValues) => {
    const payMethods = JSON.parse(
      values.pay_methods
    ) as EpayGatewayData['pay_methods']
    props.onSave({
      id: values.id.trim(),
      name: values.name.trim(),
      address: values.address.trim().replace(/\/+$/, ''),
      merchant_id: values.merchant_id.trim(),
      key: values.key.trim(),
      enabled: values.enabled,
      pay_methods: payMethods,
      schedule_enabled: values.schedule_enabled,
      schedule_start: values.schedule_enabled
        ? values.schedule_start || '00:00'
        : '',
      schedule_end: values.schedule_enabled
        ? values.schedule_end || '23:59'
        : '',
      schedule_timezone: props.editData?.schedule_timezone || 'Asia/Shanghai',
    })
    props.onOpenChange(false)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.editData ? t('Edit Epay gateway') : t('Add Epay gateway')}
      description={t(
        'Configure the provider credentials and payment methods offered through this gateway.'
      )}
      contentClassName='sm:max-w-3xl'
      contentHeight='min(72vh, 42rem)'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={EPAY_GATEWAY_FORM_ID}>
            {props.editData ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={EPAY_GATEWAY_FORM_ID}
          className='space-y-5'
          onSubmit={(event) => {
            event.stopPropagation()
            void form.handleSubmit(handleSubmit)(event)
          }}
        >
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                <div className='space-y-1'>
                  <FormLabel>{t('Enable gateway')}</FormLabel>
                  <FormDescription>
                    {t('Disabled gateways are hidden from users.')}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />

          <div className='grid gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='id'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Gateway ID')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder='primary'
                      autoComplete='off'
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Stable identifier stored with payment orders.')}
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
                  <FormLabel>{t('Gateway name')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('Primary Epay')} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='address'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Epay endpoint')}</FormLabel>
                <FormControl>
                  <Input placeholder='https://pay.example.com' {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='grid gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='merchant_id'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Epay merchant ID')}</FormLabel>
                  <FormControl>
                    <Input placeholder='10001' autoComplete='off' {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='key'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Epay secret key')}</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      autoComplete='new-password'
                      placeholder={t('Enter the gateway secret key')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {props.editData?.key === '***'
                      ? t('The saved secret remains unchanged while masked.')
                      : t('Stored securely and never returned after saving.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='space-y-3 rounded-lg border p-3'>
            <FormField
              control={form.control}
              name='schedule_enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-4'>
                  <div className='space-y-1'>
                    <FormLabel>{t('Business hours only')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Outside the window this gateway is hidden from users and cannot create orders. Callbacks still settle.'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={(checked) => {
                        field.onChange(checked)
                        if (checked) {
                          if (!form.getValues('schedule_start')) {
                            form.setValue('schedule_start', '08:00')
                          }
                          if (!form.getValues('schedule_end')) {
                            form.setValue('schedule_end', '21:00')
                          }
                        }
                      }}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            {form.watch('schedule_enabled') ? (
              <div className='grid gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='schedule_start'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Window start')}</FormLabel>
                      <FormControl>
                        <Input placeholder='08:00' {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='schedule_end'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Window end')}</FormLabel>
                      <FormControl>
                        <Input placeholder='21:00' {...field} />
                      </FormControl>
                      <FormDescription>
                        {t('24-hour HH:MM, evaluated in Asia/Shanghai.')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            ) : null}
          </div>

          <FormField
            control={form.control}
            name='pay_methods'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Gateway payment methods')}</FormLabel>
                <FormDescription>
                  {t(
                    'Configure the methods, minimum top-up, and payer fees for this gateway.'
                  )}
                </FormDescription>
                <FormControl>
                  <PaymentMethodsVisualEditor
                    value={field.value}
                    onChange={(value) =>
                      form.setValue('pay_methods', value, {
                        shouldDirty: true,
                        shouldValidate: true,
                      })
                    }
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}
