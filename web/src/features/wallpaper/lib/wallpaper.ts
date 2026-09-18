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
import { WALLPAPER_ERROR_CODES, type WallpaperErrorCode } from '../constants'

/** Mirrors the server's `model.WallpaperImageURLMaxLength`. */
export const WALLPAPER_URL_MAX_LENGTH = 2048

/**
 * Characters the server refuses because the address is painted into a CSS
 * `url("...")`. Duplicated here so the console can reject the value locally.
 */
const WALLPAPER_URL_UNSAFE_CHARS = /["'()\\<>`]/

/**
 * Client-side mirror of the server's wallpaper address validation. Returns the
 * value to submit, or `null` when the address would be refused — checking here
 * keeps the user from spending a round trip (and a rate-limit slot) on a
 * request that can only fail.
 */
export function normalizeWallpaperUrl(raw: string): string | null {
  const trimmed = raw.trim()
  if (trimmed === '' || trimmed.length > WALLPAPER_URL_MAX_LENGTH) return null
  if (WALLPAPER_URL_UNSAFE_CHARS.test(trimmed)) return null
  for (const char of trimmed) {
    const code = char.charCodeAt(0)
    if (code < 0x21 || code > 0x7e) return null
  }
  let parsed: URL
  try {
    parsed = new URL(trimmed)
  } catch {
    return null
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return null
  if (parsed.host === '') return null
  if (parsed.username !== '' || parsed.password !== '') return null
  return trimmed
}

/**
 * Quota one change costs, in the wallet units the profile reports. Mirrors the
 * server's 元 → USD → quota conversion so the console can tell the user their
 * balance is short before the request is sent; the server stays authoritative.
 * `null` means the site has not published the rates needed to answer.
 */
export function wallpaperPriceInQuota(
  priceYuan: number,
  usdExchangeRate: number,
  quotaPerUnit: number
): number | null {
  if (!(usdExchangeRate > 0) || !(quotaPerUnit > 0)) return null
  if (!(priceYuan > 0)) return 0
  return Math.ceil((priceYuan / usdExchangeRate) * quotaPerUnit)
}

/** Renders the 元 price the confirm step quotes. */
export function formatWallpaperPrice(priceYuan: number): string {
  return `¥${priceYuan.toFixed(2)}`
}

/** Reads the stable failure code a wallpaper endpoint returned, if any. */
export function getWallpaperErrorCode(
  value: unknown
): WallpaperErrorCode | null {
  const record = value as { code?: unknown } | null | undefined
  const code = record?.code
  if (typeof code !== 'string') return null
  return (WALLPAPER_ERROR_CODES as readonly string[]).includes(code)
    ? (code as WallpaperErrorCode)
    : null
}
