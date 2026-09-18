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
import { describe, expect, it } from 'vitest'

import {
  formatWallpaperPrice,
  getWallpaperErrorCode,
  normalizeWallpaperUrl,
  WALLPAPER_URL_MAX_LENGTH,
  wallpaperPriceInQuota,
} from '../wallpaper'

describe('normalizeWallpaperUrl', () => {
  it('keeps an absolute https address and trims surrounding whitespace', () => {
    expect(normalizeWallpaperUrl('  https://cdn.example.com/a.png  ')).toBe(
      'https://cdn.example.com/a.png'
    )
  })

  it('keeps http and query strings', () => {
    expect(normalizeWallpaperUrl('http://cdn.example.com/a.jpg?v=2')).toBe(
      'http://cdn.example.com/a.jpg?v=2'
    )
  })

  it.each([
    ['  ', 'an empty value'],
    ['ftp://cdn.example.com/a.png', 'a non-http scheme'],
    ['data:image/png;base64,AAAA', 'an inline data payload'],
    ['/assets/a.png', 'a relative path'],
    ['https://user:pw@cdn.example.com/a.png', 'embedded credentials'],
    ['https://cdn.example.com/a b.png', 'an inner space'],
    ['https://cdn.example.com/a(b).png', 'a parenthesis'],
    ['https://cdn.example.com/a"b.png', 'a quote'],
  ])('rejects %s (%s)', (value) => {
    expect(normalizeWallpaperUrl(value)).toBeNull()
  })

  it('rejects an address longer than the stored limit', () => {
    const oversized = `https://cdn.example.com/${'a'.repeat(WALLPAPER_URL_MAX_LENGTH)}`
    expect(normalizeWallpaperUrl(oversized)).toBeNull()
  })
})

describe('wallpaperPriceInQuota', () => {
  it('converts the configured 元 price through the exchange rate', () => {
    expect(wallpaperPriceInQuota(0.5, 8, 500000)).toBe(31250)
  })

  it('rounds up so a fractional rate never undercharges the quote', () => {
    expect(wallpaperPriceInQuota(0.5, 7.3, 500000)).toBe(34247)
  })

  it('costs nothing when the price is zero', () => {
    expect(wallpaperPriceInQuota(0, 8, 500000)).toBe(0)
  })

  it('reports unknown when the site has not published the rates', () => {
    expect(wallpaperPriceInQuota(0.5, 0, 500000)).toBeNull()
    expect(wallpaperPriceInQuota(0.5, 8, 0)).toBeNull()
  })
})

describe('formatWallpaperPrice', () => {
  it('renders a money amount with two decimals', () => {
    expect(formatWallpaperPrice(0.5)).toBe('¥0.50')
    expect(formatWallpaperPrice(1)).toBe('¥1.00')
  })
})

describe('getWallpaperErrorCode', () => {
  it('returns a known wallpaper code', () => {
    expect(
      getWallpaperErrorCode({ code: 'WALLPAPER_INSUFFICIENT_QUOTA' })
    ).toBe('WALLPAPER_INSUFFICIENT_QUOTA')
  })

  it('ignores unknown or missing codes', () => {
    expect(getWallpaperErrorCode({ code: 'SOMETHING_ELSE' })).toBeNull()
    expect(getWallpaperErrorCode({})).toBeNull()
    expect(getWallpaperErrorCode(undefined)).toBeNull()
  })
})
