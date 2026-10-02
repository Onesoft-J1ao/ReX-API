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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { getNotice } from '@/lib/api'
import { isLikelyHtml } from '@/lib/content-format'
import { useNotificationStore } from '@/stores/notification-store'

type NoticePopupProps = {
  placement: 'home' | 'dashboard'
}

export function NoticePopup(props: NoticePopupProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const markNoticeRead = useNotificationStore((state) => state.markNoticeRead)
  const lastReadNotice = useNotificationStore((state) => state.lastReadNotice)
  const [open, setOpen] = useState(false)
  const popupEnabled = Boolean(status?.notice_popup_enabled)
  const popupMode =
    status?.notice_popup_mode ||
    (status?.notice_popup_on_dashboard ? 'both' : 'home')
  const placementEnabled = popupMode === 'both' || popupMode === props.placement
  const enabled = popupEnabled && placementEnabled
  const { data } = useQuery({
    queryKey: ['notice'],
    queryFn: getNotice,
    enabled,
    staleTime: 5 * 60 * 1000,
  })
  const notice = data?.success ? (data.data || '').trim() : ''

  // Open once per unread notice. Marking it read here — before the user had
  // dismissed it — flipped `lastReadNotice` on the same render, so the guard
  // below returned null and unmounted the dialog while `open` was already true.
  // That left the Close button dead. The marker is written on dismissal instead.
  const shouldShow = enabled && Boolean(notice) && lastReadNotice !== notice
  useEffect(() => {
    if (shouldShow) setOpen(true)
  }, [shouldShow])

  if (!shouldShow) return null

  const handleOpenChange = (next: boolean) => {
    setOpen(next)
    if (!next) markNoticeRead(notice)
  }

  const isHtml = isLikelyHtml(notice)

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
      title={t('System Notice')}
      contentClassName='sm:max-w-3xl'
      contentHeight='min(65vh, 36rem)'
      footer={
        <Button type='button' variant='outline' onClick={() => handleOpenChange(false)}>
          {t('Close')}
        </Button>
      }
      showCloseButton
    >
      <RichContent
        content={notice}
        mode={isHtml ? 'html' : 'markdown'}
        htmlVariant={isHtml ? 'isolated' : undefined}
        breaks={!isHtml}
        className='min-w-0'
      />
    </Dialog>
  )
}
