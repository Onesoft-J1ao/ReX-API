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
import { renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it } from 'vitest'

import { STATUS_QUERY_KEY } from '@/lib/status-query'

import { useTopNavLinks } from '../use-top-nav-links'

describe('top navigation', () => {
  it.each([true, false])(
    'does not restore About when its legacy flag is %s',
    (about) => {
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      client.setQueryData(STATUS_QUERY_KEY, {
        HeaderNavModules: {
          about,
          bangumi: { enabled: true, requireAuth: false },
        },
      })
      const view = renderHook(() => useTopNavLinks(), {
        wrapper: (props: { children: ReactNode }) => (
          <QueryClientProvider client={client}>
            {props.children}
          </QueryClientProvider>
        ),
      })
      expect(view.result.current.some((link) => link.href === '/about')).toBe(
        false
      )
      expect(view.result.current.some((link) => link.href === '/bangumi')).toBe(
        true
      )
      view.unmount()
      client.clear()
    }
  )
})
