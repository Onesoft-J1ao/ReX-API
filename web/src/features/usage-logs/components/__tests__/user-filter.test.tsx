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
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { CommonLogsFilterBar } from '../common-logs-filter-bar'
import { UsageLogsProvider } from '../usage-logs-provider'

function FilterFixture() {
  const table = useReactTable({
    data: [],
    columns: [],
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <UsageLogsProvider>
      <CommonLogsFilterBar table={table} />
    </UsageLogsProvider>
  )
}

async function renderFilter() {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/self/groups' || url === '/api/group/') {
      return { data: { success: true, data: { default: {} } } }
    }
    return { data: { success: true, data: { quota: 0, rpm: 0, tpm: 0 } } }
  })

  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: FilterFixture,
    validateSearch: (search: Record<string, unknown>) => search,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({ initialEntries: ['/usage-logs/common'] }),
  })
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

function adminUser() {
  useAuthStore.getState().auth.setUser({ id: 1, role: 100 } as never)
}

beforeEach(() => {
  adminUser()
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.setUser(null)
})

// Triaging one account means filtering to it, and hiding a few noisy accounts
// (internal jobs, the operator's own probes) means excluding them. Both are
// admin-only, because they expose who generated which traffic.
it('filters to a single account by id after Search', async () => {
  const router = await renderFilter()

  await userEvent.type(screen.getByPlaceholderText('User ID'), '42')
  await userEvent.click(screen.getByRole('button', { name: /^Search/ }))

  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ userId: '42' })
  )
})

it('excludes the listed account ids after Search', async () => {
  const router = await renderFilter()

  await userEvent.type(
    screen.getByPlaceholderText('Exclude User IDs'),
    '7,12'
  )
  await userEvent.click(screen.getByRole('button', { name: /^Search/ }))

  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({
      excludeUserId: '7,12',
    })
  )
})

it('does not offer the account filters to a non-admin', async () => {
  useAuthStore.getState().auth.setUser({ id: 2, role: 1 } as never)
  await renderFilter()

  expect(screen.queryByPlaceholderText('User ID')).toBeNull()
  expect(screen.queryByPlaceholderText('Exclude User IDs')).toBeNull()
})
