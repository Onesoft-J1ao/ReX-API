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

import { submitPaymentForm } from '@/features/wallet/lib/payment'

import {
  getRedemptionPurchaseAmount,
  getRedemptionPurchaseOptions,
  requestRedemptionPurchase,
} from '../api'
import { RedemptionPurchaseCard } from '../components/redemption-purchase-card'

vi.mock('../api', () => ({
  getRedemptionPurchaseOptions: vi.fn(),
  getRedemptionPurchaseAmount: vi.fn(),
  requestRedemptionPurchase: vi.fn(),
}))

vi.mock('@/features/wallet/lib/payment', () => ({
  submitPaymentForm: vi.fn(),
}))

const options = {
  enabled: true,
  methods: ['alipay', 'wxpay'],
  max_count: 100,
  max_amount: 10000,
  min_amount: 10,
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(getRedemptionPurchaseAmount).mockResolvedValue('20.00')
})

describe('RedemptionPurchaseCard', () => {
  it('renders nothing while the feature is off', async () => {
    vi.mocked(getRedemptionPurchaseOptions).mockResolvedValue({
      ...options,
      enabled: false,
    })

    const { container } = render(<RedemptionPurchaseCard />)

    await waitFor(() => {
      expect(getRedemptionPurchaseOptions).toHaveBeenCalled()
    })
    expect(container).toBeEmptyDOMElement()
  })

  // The methods are the server's decision: the card must not invent one, or it
  // could offer a checkout this deployment cannot take.
  it('offers only the payment methods the server returned', async () => {
    vi.mocked(getRedemptionPurchaseOptions).mockResolvedValue({
      ...options,
      methods: ['alipay'],
    })

    render(<RedemptionPurchaseCard />)

    await waitFor(() => {
      expect(screen.getByText('Buy redemption codes')).toBeInTheDocument()
    })
    await userEvent.click(screen.getByRole('combobox'))
    expect(await screen.findByRole('option', { name: 'alipay' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'wxpay' })).not.toBeInTheDocument()
  })

  it('shows the price the server quoted for the order', async () => {
    vi.mocked(getRedemptionPurchaseOptions).mockResolvedValue(options)

    render(<RedemptionPurchaseCard />)

    await waitFor(() => {
      expect(screen.getByText(/20\.00/)).toBeInTheDocument()
    })
  })

  it('posts the order through the epay form the server handed back', async () => {
    vi.mocked(getRedemptionPurchaseOptions).mockResolvedValue({
      ...options,
      methods: ['alipay'],
    })
    vi.mocked(requestRedemptionPurchase).mockResolvedValue({
      message: 'success',
      data: { pid: '2119', sign: 'abc' },
      url: 'https://pay.example.com/submit',
    })

    render(<RedemptionPurchaseCard />)

    await waitFor(() => {
      expect(screen.getByText(/20\.00/)).toBeInTheDocument()
    })
    await userEvent.click(screen.getByRole('button', { name: 'Buy now' }))

    await waitFor(() => {
      expect(requestRedemptionPurchase).toHaveBeenCalledWith({
        unit_amount: 10,
        quantity: 1,
        payment_method: 'alipay',
      })
    })
    expect(submitPaymentForm).toHaveBeenCalledWith(
      'https://pay.example.com/submit',
      { pid: '2119', sign: 'abc' }
    )
  })

  it('opens the pay link for a provider that returns one', async () => {
    vi.mocked(getRedemptionPurchaseOptions).mockResolvedValue({
      ...options,
      methods: ['stripe'],
    })
    vi.mocked(requestRedemptionPurchase).mockResolvedValue({
      message: 'success',
      data: { pay_link: 'https://checkout.example.com/session' },
    })
    const open = vi.spyOn(window, 'open').mockReturnValue(null)

    render(<RedemptionPurchaseCard />)

    await waitFor(() => {
      expect(screen.getByText(/20\.00/)).toBeInTheDocument()
    })
    await userEvent.click(screen.getByRole('button', { name: 'Buy now' }))

    await waitFor(() => {
      expect(open).toHaveBeenCalledWith(
        'https://checkout.example.com/session',
        '_blank'
      )
    })
    expect(submitPaymentForm).not.toHaveBeenCalled()
    open.mockRestore()
  })
})
