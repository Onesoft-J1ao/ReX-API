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
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { NoticeDialog } from '@/components/notice-dialog'
import { api } from '@/lib/api'
import { useNotificationStore } from '@/stores/notification-store'

const NOTICE_STORAGE_KEY = 'notification-storage'

let noticeBody = 'Scheduled maintenance on Sunday.'
const clients: QueryClient[] = []

function resetReadState(lastReadNotice = '') {
  localStorage.clear()
  useNotificationStore.setState({
    lastReadNotice,
    readAnnouncementKeys: [],
    closedUntilDate: null,
  })
}

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <NoticeDialog />
    </QueryClientProvider>
  )
}

function persistedReadNotice(): string | undefined {
  const raw = localStorage.getItem(NOTICE_STORAGE_KEY)
  if (!raw) return undefined
  return (JSON.parse(raw) as { state?: { lastReadNotice?: string } }).state
    ?.lastReadNotice
}

beforeEach(() => {
  noticeBody = 'Scheduled maintenance on Sunday.'
  resetReadState()
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: { success: true, data: noticeBody },
  }))
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  resetReadState()
})

it('pops the notice on entry and stays quiet after the user marks it read', async () => {
  const { unmount } = renderDialog()

  const dialog = await screen.findByRole('dialog')
  expect(dialog).toHaveTextContent(noticeBody)
  expect(useNotificationStore.getState().lastReadNotice).toBe('')

  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Mark as read' })
  )

  await waitFor(() =>
    expect(useNotificationStore.getState().lastReadNotice).toBe(noticeBody)
  )
  // Persisted, not just in memory: a fresh visit must not show it again.
  expect(persistedReadNotice()).toBe(noticeBody)

  unmount()
  renderDialog()
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
})

it('pops a new announcement again after an older one was already read', async () => {
  resetReadState('Scheduled maintenance on Sunday.')
  noticeBody = 'We shipped a new pricing page.'

  renderDialog()

  const dialog = await screen.findByRole('dialog')
  expect(dialog).toHaveTextContent('We shipped a new pricing page.')
  expect(dialog).not.toHaveTextContent('Scheduled maintenance on Sunday.')
})

it('stays silent when the admin has not published a notice', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: '   ' },
  })

  renderDialog()

  await waitFor(() =>
    expect(useNotificationStore.getState().lastReadNotice).toBe('')
  )
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('treats dismissing the dialog as acknowledgement so it does not return', async () => {
  const { unmount } = renderDialog()

  await screen.findByRole('dialog')
  await userEvent.keyboard('{Escape}')

  await waitFor(() =>
    expect(useNotificationStore.getState().lastReadNotice).toBe(noticeBody)
  )

  unmount()
  renderDialog()
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
})
