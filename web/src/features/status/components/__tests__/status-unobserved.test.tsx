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
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { StatusCheckResponse, StatusModel } from '../../types'
import { StatusPanel } from '../status-panel'

const { getStatusCheck } = vi.hoisted(() => ({ getStatusCheck: vi.fn() }))
vi.mock('../../api', () => ({ getStatusCheck }))
vi.mock('@/hooks/use-status', () => ({ useStatus: () => ({ status: {} }) }))

function model(name: string, overrides: Partial<StatusModel> = {}): StatusModel {
  return {
    model: name,
    group: 'free2',
    availability: 100,
    latency_ms: 120,
    level: 'excellent',
    ok: true,
    has_active: true,
    has_data: true,
    ...overrides,
  }
}

function response(models: StatusModel[], observed = true) {
  const payload: StatusCheckResponse = {
    success: true,
    data: {
      announcement: '',
      groups: [
        {
          group: 'free2',
          availability: 100,
          avg_latency_ms: 120,
          total_probes: models.length,
          ok_probes: models.length,
          level: 'excellent',
          observed,
          models,
        },
      ],
    },
  }
  return payload
}

function renderPanel(payload: StatusCheckResponse) {
  getStatusCheck.mockResolvedValue(payload)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  return render(
    <QueryClientProvider client={client}>
      <StatusPanel />
    </QueryClientProvider>
  )
}

describe('StatusPanel unobserved models and groups', () => {
  beforeEach(() => getStatusCheck.mockReset())

  // A model the group serves but that neither a probe nor traffic reached is
  // listed with no data, rather than shown as a healthy percentage.
  it('shows a served model with no data as such', async () => {
    renderPanel(response([model('healthy-model'), model('fresh-model', {
      availability: 0,
      latency_ms: 0,
      level: 'unknown',
      ok: false,
      has_active: false,
      has_data: false,
    })]))

    expect(await screen.findByText('fresh-model')).toBeInTheDocument()
    expect(screen.getByText('No data')).toBeInTheDocument()
  })

  // ok=false is what the payload carries for an unobserved model, so counting
  // it as a failure would report every freshly configured model as broken.
  it('does not count a no-data model as failing', async () => {
    renderPanel(response([model('healthy-model'), model('fresh-model', {
      availability: 0,
      latency_ms: 0,
      level: 'unknown',
      ok: false,
      has_active: false,
      has_data: false,
    })]))

    await screen.findByText('fresh-model')
    expect(screen.queryByText(/failing/)).not.toBeInTheDocument()
  })

  it('marks an unobserved group without hiding its models', async () => {
    renderPanel(response([model('healthy-model')], false))

    expect(await screen.findByText('healthy-model')).toBeInTheDocument()
    // The group reports 100% because nothing contradicts it; the marker says
    // that figure is an assumption rather than a measurement.
    expect(screen.getAllByText('No data').length).toBeGreaterThan(0)
  })

  // An older gateway never sent `observed`, so its groups must not go grey.
  it('treats a missing observed flag as observed', async () => {
    const payload = response([model('healthy-model')])
    delete (payload.data.groups[0] as { observed?: boolean }).observed
    renderPanel(payload)

    await screen.findByText('healthy-model')
    // Measured latency shows, which only happens on the observed branch.
    // Both the group header and the model row carry it.
    expect(screen.getAllByText('120ms').length).toBeGreaterThan(0)
  })

  it('puts a no-data model after every measured one in the preview', async () => {
    const many = Array.from({ length: 6 }, (_, index) =>
      model(`measured-${index}`)
    )
    many.push(
      model('fresh-model', {
        availability: 0,
        latency_ms: 0,
        level: 'unknown',
        ok: false,
        has_active: false,
        has_data: false,
      })
    )
    renderPanel(response(many))

    await screen.findByText('measured-0')
    expect(screen.getByText('measured-5')).toBeInTheDocument()
    // The preview holds six rows and the no-data model ranks below all of them.
    expect(screen.queryByText('fresh-model')).not.toBeInTheDocument()

    await userEvent.click(
      screen.getByRole('button', { name: /more models/ })
    )
    expect(screen.getByText('fresh-model')).toBeInTheDocument()
  })
})