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
/** Stable error codes the wallpaper endpoints return alongside the message. */
export const WALLPAPER_ERROR_CODES = [
  'WALLPAPER_DISABLED',
  'WALLPAPER_MONTHLY_LIMIT_REACHED',
  'WALLPAPER_INSUFFICIENT_QUOTA',
  'WALLPAPER_UNCHANGED',
  'WALLPAPER_INVALID_URL',
] as const

export type WallpaperErrorCode = (typeof WALLPAPER_ERROR_CODES)[number]

/**
 * Code → i18n key. Every entry is a key registered in
 * `src/i18n/static-keys.ts`, because the value is picked at runtime.
 */
export const WALLPAPER_ERROR_MESSAGES: Record<WallpaperErrorCode, string> = {
  WALLPAPER_DISABLED: 'Custom wallpaper is not available on this site.',
  WALLPAPER_MONTHLY_LIMIT_REACHED:
    'You have used all your wallpaper changes for this month.',
  WALLPAPER_INSUFFICIENT_QUOTA:
    'Your balance is too low for this wallpaper change.',
  WALLPAPER_UNCHANGED:
    'That is already your wallpaper, so nothing was charged.',
  WALLPAPER_INVALID_URL:
    'Enter an absolute http(s) image address without spaces or quotes.',
}
