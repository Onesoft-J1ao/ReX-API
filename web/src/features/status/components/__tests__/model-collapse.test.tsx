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

/** Mirrors MODEL_PREVIEW_COUNT in status-panel.tsx. */
const PREVIEW_COUNT = 6

function healthyModel(name: string): StatusModel {
  return {
    model: name,
    group: 'free2',
    availability: 100,
    latency_ms: 100,
    level: 'excellent',
    ok: true,
    has_active: true,
  }
}

function responseWith(models: StatusModel[]): StatusCheckResponse {
  return {
    success: true,
    data: {
      announcement: '',
      groups: [
        {
          group: 'free2',
          availability: 100,
          avg_latency_ms: 100,
          total_probes: models.length,
          ok_probes: models.length,
          level: 'excellent',
          models,
        },
      ],
    },
  }
}

function renderPanel(models: StatusModel[]) {
  getStatusCheck.mockResolvedValue(responseWith(models))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <StatusPanel />
    </QueryClientProvider>
  )
}

describe('StatusPanel model list', () => {
  beforeEach(() => {
    getStatusCheck.mockReset()
  })

  it('collapses a long model list behind a toggle', async () => {
    const models = Array.from({ length: 10 }, (_, index) =>
      healthyModel(`m-${index}`)
    )
    renderPanel(models)

    expect(await screen.findByText('m-0')).toBeInTheDocument()
    expect(screen.getByText(`m-${PREVIEW_COUNT - 1}`)).toBeInTheDocument()
    expect(screen.queryByText(`m-${PREVIEW_COUNT}`)).not.toBeInTheDocument()

    await userEvent.click(
      screen.getByRole('button', { name: /more models/ })
    )
    expect(screen.getByText(`m-${models.length - 1}`)).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /more models/ })
    ).not.toBeInTheDocument()

    await userEvent.click(
      screen.getByRole('button', { name: /fewer models/ })
    )
    expect(screen.queryByText(`m-${PREVIEW_COUNT}`)).not.toBeInTheDocument()
  })

  it('offers no toggle when every model already fits', async () => {
    const models = Array.from({ length: PREVIEW_COUNT }, (_, index) =>
      healthyModel(`m-${index}`)
    )
    renderPanel(models)

    expect(await screen.findByText(`m-${PREVIEW_COUNT - 1}`)).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /more models|fewer models/ })
    ).not.toBeInTheDocument()
  })
})
