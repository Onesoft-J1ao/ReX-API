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
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { api } from '@/lib/api'
import { createAppQueryClient } from '@/lib/query-client'
import { useAuthStore } from '@/stores/auth-store'

import type { WallpaperStatus } from '../../types'
import { WallpaperConfig } from '../wallpaper-config'
import { WallpaperSync } from '../wallpaper-sync'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: { usd_exchange_rate: 8, quota_per_unit: 500000 },
    loading: false,
    error: null,
  }),
}))

const STORED_WALLPAPER = 'https://cdn.example.com/current.png'
const PRICE_IN_QUOTA = 31250 // 0.5 元 at ¥8 = $1 and $1 = 500000 quota

const originalAuth = useAuthStore.getState().auth

function wallpaperStatus(
  overrides: Partial<WallpaperStatus> = {}
): WallpaperStatus {
  return {
    enabled: true,
    price_yuan: 0.5,
    monthly_limit: 3,
    wallpaper_url: STORED_WALLPAPER,
    period: '2026-09',
    changes_used: 1,
    changes_remaining: 2,
    ...overrides,
  }
}

function mockStoredWallpaper(status: WallpaperStatus) {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: status },
  })
}

function renderWallpaperSettings() {
  const client = createAppQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <ThemeCustomizationProvider>
        <WallpaperSync />
        <WallpaperConfig />
      </ThemeCustomizationProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useAuthStore.setState({
    auth: {
      ...originalAuth,
      user: { id: 1, username: 'alice', role: 1, quota: 1_000_000 },
    },
  })
})

afterEach(() => {
  useAuthStore.setState({ auth: originalAuth })
  document.body.removeAttribute('data-theme-wallpaper')
  document.body.style.removeProperty('--wallpaper-image')
})

describe('wallpaper settings', () => {
  it('paints the stored wallpaper on the console background', async () => {
    mockStoredWallpaper(wallpaperStatus())
    renderWallpaperSettings()

    await waitFor(() => {
      expect(document.body.getAttribute('data-theme-wallpaper')).toBe('custom')
    })
    expect(document.body.style.getPropertyValue('--wallpaper-image')).toBe(
      `url("${STORED_WALLPAPER}")`
    )
  })

  it('quotes the price and the remaining changes without charging anything', async () => {
    const put = vi.spyOn(api, 'put')
    mockStoredWallpaper(wallpaperStatus())
    renderWallpaperSettings()

    expect(
      await screen.findByText('One change costs ¥0.50.')
    ).toBeInTheDocument()
    expect(
      screen.getByText('1 of 3 changes used this month')
    ).toBeInTheDocument()
    expect(put).not.toHaveBeenCalled()

    // Opening the section is never a charge, and an empty address cannot buy
    // one: the action stays disabled until a usable address is entered.
    expect(
      screen.getByRole('button', { name: 'Change wallpaper' })
    ).toBeDisabled()
  })

  it('charges only after the user confirms the quoted price', async () => {
    const user = userEvent.setup()
    mockStoredWallpaper(wallpaperStatus())
    const put = vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: true, data: wallpaperStatus({ changes_used: 2 }) },
    })
    renderWallpaperSettings()

    await user.type(
      await screen.findByLabelText('Wallpaper image address'),
      '  https://cdn.example.com/next.png  '
    )
    await user.click(screen.getByRole('button', { name: 'Change wallpaper' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(
      within(dialog).getByText(
        'This change costs ¥0.50 and leaves 1 of 3 changes for this month.'
      )
    ).toBeInTheDocument()
    expect(put).not.toHaveBeenCalled()

    await user.click(
      within(dialog).getByRole('button', { name: 'Change wallpaper' })
    )

    await waitFor(() => {
      expect(put).toHaveBeenCalledWith('/api/user/wallpaper', {
        wallpaper_url: 'https://cdn.example.com/next.png',
      })
    })
    expect(toast.success).toHaveBeenCalled()
  })

  it('refuses to buy a change once the month is used up', async () => {
    mockStoredWallpaper(
      wallpaperStatus({ changes_used: 3, changes_remaining: 0 })
    )
    renderWallpaperSettings()

    const user = userEvent.setup()
    await user.type(
      await screen.findByLabelText('Wallpaper image address'),
      'https://cdn.example.com/next.png'
    )

    expect(
      screen.getByText(
        'You have used all your wallpaper changes for this month.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Change wallpaper' })
    ).toBeDisabled()
  })

  it('refuses to buy a change the balance cannot cover', async () => {
    useAuthStore.setState({
      auth: {
        ...originalAuth,
        user: { id: 1, username: 'alice', role: 1, quota: PRICE_IN_QUOTA - 1 },
      },
    })
    mockStoredWallpaper(wallpaperStatus())
    renderWallpaperSettings()

    const user = userEvent.setup()
    await user.type(
      await screen.findByLabelText('Wallpaper image address'),
      'https://cdn.example.com/next.png'
    )

    expect(
      screen.getByText('Your balance is too low for this wallpaper change.')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Change wallpaper' })
    ).toBeDisabled()
  })

  it.each([
    [
      'WALLPAPER_MONTHLY_LIMIT_REACHED',
      'You have used all your wallpaper changes for this month.',
    ],
    [
      'WALLPAPER_INSUFFICIENT_QUOTA',
      'Your balance is too low for this wallpaper change.',
    ],
    [
      'WALLPAPER_UNCHANGED',
      'That is already your wallpaper, so nothing was charged.',
    ],
  ])('surfaces %s with its own message', async (code, message) => {
    const user = userEvent.setup()
    mockStoredWallpaper(wallpaperStatus())
    vi.spyOn(api, 'put').mockRejectedValue(
      new Error('server refused', {
        cause: { success: false, code, message: 'server refused' },
      })
    )
    renderWallpaperSettings()

    await user.type(
      await screen.findByLabelText('Wallpaper image address'),
      'https://cdn.example.com/next.png'
    )
    await user.click(screen.getByRole('button', { name: 'Change wallpaper' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Change wallpaper' })
    )

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(message)
    })
    // The dialog closes so the user can fix the address and retry.
    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    })
  })
})
