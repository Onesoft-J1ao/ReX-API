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
export interface WallpaperStatus {
  /** Whether the deployment offers the feature at all. */
  enabled: boolean
  /** Price of one change, in 元. */
  price_yuan: number
  /** Changes the current calendar month allows. `<= 0` means unlimited. */
  monthly_limit: number
  /** The caller's current wallpaper address, empty when none is set. */
  wallpaper_url: string
  /** Calendar month the counter belongs to, `YYYY-MM`. */
  period: string
  changes_used: number
  /** Remaining changes; `-1` means unlimited. */
  changes_remaining: number
}

export interface ApiResponse<T = unknown> {
  success: boolean
  /** Stable failure identifier, present on wallpaper errors. */
  code?: string
  message?: string
  data?: T
}

/** What a Wallpaper Engine (Steam Workshop) link resolves to. */
export interface WallpaperResolveResult {
  item_id: string
  /** The item's own title, shown so the user can confirm the right one. */
  title: string
  /** Preview image of the item, ready for the wallpaper address field. */
  image_url: string
  app_id: number
  /** False for a Workshop item that is not a Wallpaper Engine wallpaper. */
  wallpaper_engine: boolean
}
