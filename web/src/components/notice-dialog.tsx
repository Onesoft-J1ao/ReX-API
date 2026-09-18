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
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { getNotice } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useNotificationStore } from '@/stores/notification-store'

/**
 * Shows the site notice once per announcement.
 *
 * The read marker is keyed by the notice body, not by a boolean, so editing the
 * notice in the admin console (or publishing a new one) pops the dialog again
 * while an unchanged notice stays silent. The marker lives in
 * `notification-store`'s persisted `lastReadNotice`, which the header bell also
 * reads, so dismissing here clears the unread badge there too.
 */
export function NoticeDialog() {
  const { t } = useTranslation()
  const lastReadNotice = useNotificationStore((state) => state.lastReadNotice)
  const markNoticeRead = useNotificationStore((state) => state.markNoticeRead)

  const { data: noticeResponse } = useQuery({
    // Shares the header bell's cache entry, so a visitor never pays for two
    // /api/notice requests.
    queryKey: ['notice'],
    queryFn: async () => requireServerSuccess(await getNotice()),
    staleTime: 1000 * 60 * 5,
  })

  const noticeContent = noticeResponse?.success
    ? (noticeResponse.data || '').trim()
    : ''
  const open = Boolean(noticeContent) && noticeContent !== lastReadNotice

  // Every dismissal path (the button, Escape, or a backdrop click) acknowledges
  // the announcement. Trapping the user behind the dialog would be worse than an
  // occasionally missed banner.
  const handleOpenChange = (next: boolean) => {
    if (next) return
    if (!noticeContent) return
    markNoticeRead(noticeContent)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
      title={t('Notice')}
      description={t('Latest platform updates and notices')}
      contentHeight='auto'
      bodyClassName='text-sm'
      footer={
        <Button size='sm' onClick={() => handleOpenChange(false)}>
          {t('Mark as read')}
        </Button>
      }
    >
      <RichContent breaks content={noticeContent} />
    </Dialog>
  )
}
