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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  applyPriceSentinelEdit,
  syncPriceSentinelPrices,
  validatePriceSentinelToken,
} from '../../api'
import type { PriceSentinelEditSession } from '../../types'
import { PriceSentinelEditor } from '../price-sentinel-editor'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

// The page's only outside dependency is the HTTP calls, so they are the
// boundary this test replaces.
vi.mock('../../api', () => ({
  validatePriceSentinelToken: vi.fn(),
  applyPriceSentinelEdit: vi.fn(),
  syncPriceSentinelPrices: vi.fn(),
  getPriceSentinelReport: vi.fn(),
  runPriceSentinelNow: vi.fn(),
}))

const validateMock = vi.mocked(validatePriceSentinelToken)
const applyMock = vi.mocked(applyPriceSentinelEdit)
const syncMock = vi.mocked(syncPriceSentinelPrices)

const LINK_TOKEN = 'mailed-link-token'

function session(
  overrides: Partial<PriceSentinelEditSession> = {}
): PriceSentinelEditSession {
  return {
    report: {
      id: 1,
      checked_at: 1_790_000_000,
      upstream_url: 'https://upstream.example/api/pricing',
      compared: 158,
      changed: 1,
      higher_price: 1,
      lower_price: 0,
      email_sent_at: 1_790_000_100,
    },
    changes: [
      {
        model_name: 'grok-4.6',
        field: 'model_ratio',
        ours: 0.08,
        theirs: 0.1,
        percent: 25,
        groups: ['default'],
      },
    ],
    expires_at: 1_790_086_400,
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  validateMock.mockResolvedValue(session())
  applyMock.mockResolvedValue({
    model_name: 'grok-4.6',
    field: 'model_ratio',
    value: 0.1,
  })
  syncMock.mockResolvedValue({ applied: 1, skipped: 0, margin_percent: 0.2 })
})

describe('price sentinel edit page', () => {
  it('shows the reported differences once the link validates', async () => {
    render(<PriceSentinelEditor token={LINK_TOKEN} />)

    expect(await screen.findByText('grok-4.6')).toBeInTheDocument()
    expect(screen.getByText('model_ratio')).toBeInTheDocument()
    expect(validateMock).toHaveBeenCalledWith(LINK_TOKEN)
  })

  // The link proves the request came from us; the pasted token is what
  // authorises the write. A leaked link on its own must not be enough.
  it('sends the pasted admin token with the edit', async () => {
    const user = userEvent.setup()
    render(<PriceSentinelEditor token={LINK_TOKEN} />)

    await screen.findByText('grok-4.6')
    await user.type(
      screen.getByLabelText('Admin access token'),
      'admin-access-token'
    )
    await user.click(screen.getByRole('button', { name: 'Apply' }))

    await waitFor(() => {
      expect(applyMock).toHaveBeenCalledWith({
        token: LINK_TOKEN,
        adminToken: 'admin-access-token',
        modelName: 'grok-4.6',
        field: 'model_ratio',
        value: 0.1,
      })
    })
  })

  it('reports a link that cannot be used instead of an empty table', async () => {
    validateMock.mockRejectedValue({
      response: { data: { message: 'link-expired' } },
    })

    render(<PriceSentinelEditor token={LINK_TOKEN} />)

    expect(await screen.findByText('link-expired')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('drops a row once its edit has been applied', async () => {
    const user = userEvent.setup()
    render(<PriceSentinelEditor token={LINK_TOKEN} />)

    await screen.findByText('grok-4.6')
    await user.click(screen.getByRole('button', { name: 'Apply' }))

    await waitFor(() => {
      expect(screen.queryByText('grok-4.6')).not.toBeInTheDocument()
    })
    expect(await screen.findByText('Nothing left to apply')).toBeInTheDocument()
  })

  // The page takes the margin as a percentage and the API takes a fraction, so
  // the conversion is the one thing a bulk write can silently get wrong.
  it('sends the typed margin as a fraction', async () => {
    const user = userEvent.setup()
    render(<PriceSentinelEditor token={LINK_TOKEN} />)

    await screen.findByText('grok-4.6')
    await user.type(
      screen.getByLabelText('Admin access token'),
      'admin-access-token'
    )
    await user.type(screen.getByLabelText('Gross margin (%)'), '25')
    await user.click(screen.getByRole('button', { name: 'Sync all' }))

    await waitFor(() => {
      expect(syncMock).toHaveBeenCalledWith({
        token: LINK_TOKEN,
        adminToken: 'admin-access-token',
        marginPercent: 0.25,
      })
    })
  })

  // An empty field means "use the margin configured on the server", which is not
  // the same as the meaningful value zero (follow the upstream exactly).
  it('omits the margin when the field is left empty', async () => {
    const user = userEvent.setup()
    render(<PriceSentinelEditor token={LINK_TOKEN} />)

    await screen.findByText('grok-4.6')
    await user.click(screen.getByRole('button', { name: 'Sync all' }))

    await waitFor(() => {
      expect(syncMock).toHaveBeenCalledWith({
        token: LINK_TOKEN,
        adminToken: '',
        marginPercent: undefined,
      })
    })
  })

  // A report lists models we never priced; the sync leaves them alone, and the
  // operator has to be told rather than left to assume they were written.
  it('reports how many unpriced models the sync skipped', async () => {
    const user = userEvent.setup()
    syncMock.mockResolvedValue({ applied: 4, skipped: 3, margin_percent: 0.2 })
    render(<PriceSentinelEditor token={LINK_TOKEN} />)

    await screen.findByText('grok-4.6')
    await user.click(screen.getByRole('button', { name: 'Sync all' }))

    // The test i18n setup has no resources, so t() returns the key with the
    // placeholders interpolated.
    await waitFor(() => {
      expect(vi.mocked(toast.success)).toHaveBeenCalledWith(
        'Applied 4 prices and skipped 3 unpriced models.'
      )
    })
    expect(await screen.findByText('Nothing left to apply')).toBeInTheDocument()
  })
})
