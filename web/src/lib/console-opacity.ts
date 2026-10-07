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
/**
 * Per-browser override for how much of the backdrop shows through the console
 * surfaces.
 *
 * The administrator sets the baseline in `console_setting.background_blur_opacity`;
 * this lets a viewer make the panels sheerer or denser without changing the
 * site-wide value. `null` means "use the configured value".
 */
export const CONSOLE_OPACITY_COOKIE_KEY = 'console_opacity'

/** Sentinel meaning the operator's configured value applies. */
export const CONSOLE_OPACITY_DEFAULT: number | null = null

export const CONSOLE_OPACITY_MIN = 0
export const CONSOLE_OPACITY_MAX = 100
export const CONSOLE_OPACITY_STEP = 5

export function clampConsoleOpacity(value: number): number {
  if (!Number.isFinite(value)) return CONSOLE_OPACITY_MIN
  return Math.min(CONSOLE_OPACITY_MAX, Math.max(CONSOLE_OPACITY_MIN, Math.round(value)))
}

/** Parses the stored cookie. Anything unreadable falls back to the operator's value. */
export function parseConsoleOpacity(raw: string | undefined): number | null {
  if (!raw) return CONSOLE_OPACITY_DEFAULT
  const parsed = Number.parseInt(raw, 10)
  if (Number.isNaN(parsed)) return CONSOLE_OPACITY_DEFAULT
  return clampConsoleOpacity(parsed)
}
