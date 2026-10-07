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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { BangumiRanking } from '../components/bangumi-ranking'

const PAGE_SIZE = 24

let client: QueryClient
const select = vi.fn()

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  select.mockReset()
})
afterEach(() => client.clear())

function page(items: { id: number; name: string }[], total: number) {
  return { data: { data: items, total, limit: PAGE_SIZE, offset: 0 } }
}

function mount() {
  const request = vi.spyOn(api, 'get').mockImplementation(async (_url, config) => {
    const requested = Number(config?.params?.page ?? 1)
    // Two entries on the first page and one on the second, so a rank that came
    // from the page index rather than the response's own page size would still
    // be caught by the second page's first card.
    if (requested === 1) {
      return page(
        [
          { id: 1, name: 'First anime' },
          { id: 2, name: 'Second anime' },
        ],
        PAGE_SIZE * 2
      )
    }
    return page([{ id: 3, name: 'Twenty-fifth anime' }], PAGE_SIZE * 2)
  })
  render(
    <QueryClientProvider client={client}>
      <BangumiRanking onSelect={select} />
    </QueryClientProvider>
  )
  return request
}

describe('anime ranking', () => {
  it('numbers the first page from one and continues the numbering on the next page', async () => {
    mount()
    expect(await screen.findByText('#1')).toBeVisible()
    expect(screen.getByText('#2')).toBeVisible()

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByText('#25')).toBeVisible()
    expect(screen.queryByText('#1')).toBeNull()

    // The rank is a position, not part of the entry, so opening one hands the
    // subject itself to the caller.
    fireEvent.click(
      screen.getByRole('button', { name: 'View Twenty-fifth anime' })
    )
    expect(select).toHaveBeenCalledWith(
      expect.objectContaining({ id: 3, name: 'Twenty-fifth anime' })
    )
  })

  it('bounds paging to the last page rather than letting a caller walk the catalogue', async () => {
    mount()
    await screen.findByText('#1')
    // total / page size is two pages, so the second page is the last one.
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findByText('#25')
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeEnabled()
  })

  it('shows an error state with a retry when the ranking cannot be loaded', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new Error('offline'))
    render(
      <QueryClientProvider client={client}>
        <BangumiRanking onSelect={select} />
      </QueryClientProvider>
    )
    expect(
      await screen.findByText('Could not reach the anime source. Try again later.')
    ).toBeVisible()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible()
    )
  })
})
