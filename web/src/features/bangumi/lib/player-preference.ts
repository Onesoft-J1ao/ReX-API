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
 * Small viewer preferences that are not viewing progress: the playback speed
 * and the danmaku block list. Both are the viewer's own and belong in their
 * storage rather than on the server.
 */
/** The playback speeds the player offers. 1 is first so the default reads as the norm. */
export const PLAYER_SPEEDS = [1, 0.5, 0.75, 1.25, 1.5, 1.75, 2] as const

export function readPreference(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

export function writePreference(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // A disabled or full storage loses the preference; playback continues.
  }
}
