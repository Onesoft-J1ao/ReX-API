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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { applyPriceSentinelEdit, validatePriceSentinelToken } from '../../api'
import type { PriceSentinelEditSession } from '../../types'
import { PriceSentinelEditor } from '../price-sentinel-editor'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

// The page's only outside dependency is the two HTTP calls, so they are the
// boundary this test replaces.
vi.mock('../../api', () => ({
  validatePriceSentinelToken: vi.fn(),
  applyPriceSentinelEdit: vi.fn(),
  getPriceSentinelReport: vi.fn(),
  runPriceSentinelNow: vi.fn(),
}))

const validateMock = vi.mocked(validatePriceSentinelToken)
const applyMock = vi.mocked(applyPriceSentinelEdit)

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
})
