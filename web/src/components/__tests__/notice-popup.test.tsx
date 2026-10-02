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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { NoticePopup } from '@/components/notice-popup'
import { api } from '@/lib/api'
import { useNotificationStore } from '@/stores/notification-store'

const NOTICE = 'Scheduled maintenance on Sunday.'

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      notice_popup_enabled: true,
      notice_popup_mode: 'both',
    },
  }),
}))

const clients: QueryClient[] = []

function renderPopup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <NoticePopup placement='dashboard' />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
  useNotificationStore.setState({
    lastReadNotice: '',
    readAnnouncementKeys: [],
    closedUntilDate: null,
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: NOTICE },
  } as never)
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
})

it('keeps the dialog open until the user dismisses it', async () => {
  renderPopup()

  // Opening must not mark the notice read: doing so flipped `lastReadNotice` on
  // the same render, unmounting the dialog while `open` was already true and
  // leaving the Close button dead.
  const dialog = await screen.findByRole('dialog')
  expect(useNotificationStore.getState().lastReadNotice).toBe('')
  expect(dialog).toBeInTheDocument()
})

it('closes on the Close button and records the notice as read', async () => {
  renderPopup()

  const dialog = await screen.findByRole('dialog')
  // The dialog has two Close affordances (the footer button and the header X);
  // the footer one is the labelled control under test.
  await userEvent.click(
    within(dialog).getAllByRole('button', { name: 'Close' })[0]
  )

  await waitFor(() => expect(dialog).not.toBeInTheDocument())
  expect(useNotificationStore.getState().lastReadNotice).toBe(NOTICE)
})

it('stays closed after the user has read the notice', async () => {
  useNotificationStore.setState({ lastReadNotice: NOTICE })

  renderPopup()

  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
})
