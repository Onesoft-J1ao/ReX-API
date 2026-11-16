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
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { StatusCheckResponse, StatusGroup, StatusModel } from '../../types'
import { StatusPanel } from '../status-panel'

const { getStatusCheck } = vi.hoisted(() => ({ getStatusCheck: vi.fn() }))

vi.mock('../../api', () => ({ getStatusCheck }))

function model(name: string): StatusModel {
  return {
    model: name,
    group: 'default',
    availability: 100,
    latency_ms: 120,
    level: 'excellent',
    ok: true,
    has_active: true,
    has_data: true,
  }
}

function group(
  name: string,
  models: StatusModel[] = [model('m-0')]
): StatusGroup {
  return {
    group: name,
    availability: 100,
    avg_latency_ms: 120,
    total_probes: models.length,
    ok_probes: models.length,
    level: 'excellent',
    models,
  }
}

function responseWith(groups: StatusGroup[]): StatusCheckResponse {
  return { success: true, data: { announcement: '', groups } }
}

function renderPanel(groups: StatusGroup[]) {
  getStatusCheck.mockResolvedValue(responseWith(groups))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  return render(
    <QueryClientProvider client={client}>
      <StatusPanel />
    </QueryClientProvider>
  )
}

describe('StatusPanel group and model names', () => {
  beforeEach(() => getStatusCheck.mockReset())

  // The page must not cap the group list client-side: every group the API
  // returns gets a card, however many that is.
  it('renders every group returned by the status endpoint', async () => {
    const groups = Array.from({ length: 20 }, (_, index) =>
      group(`group-${index}`)
    )
    renderPanel(groups)

    expect(
      await screen.findByRole('heading', { level: 3, name: 'group-19' })
    ).toBeInTheDocument()
    expect(screen.getAllByRole('heading', { level: 3 })).toHaveLength(20)
  })

  // Long group names used to be ellipsized (`Codex…`), which made same-prefix
  // groups indistinguishable. The full name must be visible and available as a
  // hover title even when the card has to wrap it.
  it('shows the full group name and exposes it as a title', async () => {
    renderPanel([group('Codex不降智高缓')])

    const heading = await screen.findByRole('heading', {
      level: 3,
      name: 'Codex不降智高缓',
    })
    expect(heading).toHaveTextContent('Codex不降智高缓')
    expect(heading).toHaveAttribute('title', 'Codex不降智高缓')
  })

  // The multiplier is the whole point of the badge: a card that hides it, or
  // prints `1` for a group the server did not report, would show a price the
  // group does not charge. Zero is free, and it has to read that way.
  it('prints the group multiplier and treats zero as free', async () => {
    const priced = { ...group('ds套餐'), group_ratio: 0.04 }
    const free = { ...group('free1'), group_ratio: 0 }
    renderPanel([priced, free])

    expect(await screen.findByText('0.04x')).toBeInTheDocument()
    expect(screen.getByText('free')).toBeInTheDocument()
  })

  // An older gateway never sends group_ratio. Inventing `1x` for it would claim
  // a price nobody measured, so the badge is simply absent.
  it('hides the multiplier when the gateway does not report it', async () => {
    renderPanel([group('legacy')])

    await screen.findByRole('heading', { level: 3, name: 'legacy' })
    expect(screen.queryByTitle('Group multiplier')).toBeNull()
  })

  it('shows the full model name and exposes it as a title', async () => {
    renderPanel([group('ClaudeMax', [model('claude-opus-4-5-20251101')])])

    const name = await screen.findByText('claude-opus-4-5-20251101')
    expect(name).toHaveAttribute('title', 'claude-opus-4-5-20251101')
  })
})
