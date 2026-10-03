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
import { ImagePlus } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { RichContent } from '@/components/rich-content'
import { isLikelyHtml } from '@/lib/content-format'
import { readFileAsDataUrl } from '@/lib/file-to-data-url'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

/**
 * A picked image is embedded in the notice as a data URL, and the notice is
 * served to every visitor, so a large one would be paid for on every page load.
 * The cap keeps a banner-sized image comfortably inside the option.
 */
const NOTICE_IMAGE_MAX_BYTES = 1024 * 1024

/**
 * Render the notice body as the visitor will see it.
 *
 * An inserted image is a data URL sitting in the middle of the textarea, so the
 * only way to check what a banner looks like — or that the markup around it is
 * right — is to render it. The same component the popup uses, so the preview
 * cannot drift from the real thing.
 */
function NoticePreview(props: { content: string }) {
  const { t } = useTranslation()
  const content = props.content.trim()
  if (!content) return null
  const isHtml = isLikelyHtml(content)

  return (
    <div className='space-y-2'>
      <div className='text-sm font-medium'>{t('Preview')}</div>
      <div className='bg-muted/30 max-h-80 overflow-auto rounded-md border px-4 py-3'>
        <RichContent
          content={content}
          mode={isHtml ? 'html' : 'markdown'}
          htmlVariant={isHtml ? 'isolated' : undefined}
          breaks={!isHtml}
          className='min-w-0'
        />
      </div>
    </div>
  )
}

const noticeSchema = z.object({
  Notice: z.string().optional(),
  NoticePopupEnabled: z.boolean(),
  NoticePopupMode: z.enum(['home', 'dashboard', 'both']),
  NoticeHeaderButtonMode: z.enum(['popover', 'dialog']),
})

type NoticeFormValues = z.infer<typeof noticeSchema>

type NoticeSectionProps = {
  defaultValues: NoticeFormValues
}

export function NoticeSection({ defaultValues }: NoticeSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const defaultNotice = defaultValues.Notice ?? ''
  const defaultPopupEnabled = defaultValues.NoticePopupEnabled
  const defaultPopupMode = defaultValues.NoticePopupMode
  const defaultHeaderButtonMode = defaultValues.NoticeHeaderButtonMode
  const form = useForm<NoticeFormValues>({
    resolver: zodResolver(noticeSchema),
    defaultValues: {
      Notice: defaultNotice,
      NoticePopupEnabled: defaultPopupEnabled,
      NoticePopupMode: defaultPopupMode,
      NoticeHeaderButtonMode: defaultHeaderButtonMode,
    },
  })

  useEffect(() => {
    form.reset({
      Notice: defaultNotice,
      NoticePopupEnabled: defaultPopupEnabled,
      NoticePopupMode: defaultPopupMode,
      NoticeHeaderButtonMode: defaultHeaderButtonMode,
    })
  }, [
    defaultHeaderButtonMode,
    defaultNotice,
    defaultPopupEnabled,
    defaultPopupMode,
    form,
  ])

  const onSubmit = async (values: NoticeFormValues) => {
    const normalizedValues: NoticeFormValues = {
      ...values,
      Notice: values.Notice ?? '',
    }
    const initialValues: NoticeFormValues = {
      Notice: defaultNotice,
      NoticePopupEnabled: defaultPopupEnabled,
      NoticePopupMode: defaultPopupMode,
      NoticeHeaderButtonMode: defaultHeaderButtonMode,
    }
    const updates = Object.entries(normalizedValues).filter(
      ([key, value]) => value !== initialValues[key as keyof NoticeFormValues]
    )

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value })
    }
  }

  const popupEnabled = form.watch('NoticePopupEnabled')
  const noticeValue = form.watch('Notice') ?? ''

  const noticeFieldRef = useRef<HTMLTextAreaElement | null>(null)
  const imageInputRef = useRef<HTMLInputElement | null>(null)

  const insertNoticeImage = async (file: File) => {
    if (file.size > NOTICE_IMAGE_MAX_BYTES) {
      toast.error(
        t('The image is larger than {{limit}} and was not inserted.', {
          limit: '1 MB',
        })
      )
      return
    }
    const dataUrl = await readFileAsDataUrl(file)
    if (!dataUrl) return
    // Insert where the caret is so the operator can place a banner between two
    // paragraphs; the notice renders HTML, so the tag is written as HTML.
    const notice = form.getValues('Notice') ?? ''
    const at = noticeFieldRef.current?.selectionStart ?? notice.length
    const tag = `<img src="${dataUrl}" alt="" />`
    form.setValue('Notice', notice.slice(0, at) + tag + notice.slice(at), {
      shouldDirty: true,
    })
    const caret = at + tag.length
    requestAnimationFrame(() => {
      noticeFieldRef.current?.focus()
      noticeFieldRef.current?.setSelectionRange(caret, caret)
    })
  }

  return (
    <SettingsSection title={t('System Notice')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save notice'
          />
          <FormField
            control={form.control}
            name='Notice'
            render={({ field }) => (
              <FormItem>
                <div className='flex flex-wrap items-center justify-between gap-3'>
                  <FormLabel>{t('Announcement content')}</FormLabel>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => imageInputRef.current?.click()}
                  >
                    <ImagePlus />
                    {t('Insert image')}
                  </Button>
                </div>
                <FormControl>
                  <Textarea
                    rows={8}
                    placeholder={t(
                      'Planned maintenance on Friday at 22:00 UTC...'
                    )}
                    {...field}
                    ref={(element) => {
                      field.ref(element)
                      noticeFieldRef.current = element
                    }}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'An inserted image travels inside the notice itself, so every visitor downloads it. Prefer a small file, or link to one you host.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <input
            ref={imageInputRef}
            type='file'
            accept='image/*'
            className='hidden'
            onChange={(event) => {
              const file = event.target.files?.[0]
              // Clear so picking the same file twice fires change again.
              event.target.value = ''
              if (file) void insertNoticeImage(file)
            }}
          />
          <NoticePreview content={noticeValue} />

          <FormField
            control={form.control}
            name='NoticePopupEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Show notice as a popup')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Show the system notice whenever users open the home page'
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
            name='NoticePopupMode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Popup display mode')}</FormLabel>
                <FormControl>
                  <Select
                    value={field.value}
                    onValueChange={field.onChange}
                    disabled={!popupEnabled}
                  >
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='home'>
                          {t('Home page only')}
                        </SelectItem>
                        <SelectItem value='dashboard'>
                          {t('Overview dashboard only')}
                        </SelectItem>
                        <SelectItem value='both'>
                          {t('Home page and overview dashboard')}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
                <FormDescription>
                  {t('Choose where the notice popup appears')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='NoticeHeaderButtonMode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Header announcement button behavior')}
                </FormLabel>
                <FormControl>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='popover'>
                          {t('Show below the button')}
                        </SelectItem>
                        <SelectItem value='dialog'>
                          {t('Open a dialog')}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
                <FormDescription>
                  {t(
                    'Choose how the announcement center opens from the header'
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
