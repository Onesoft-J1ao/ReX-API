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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { ApiResponse, WallpaperStatus } from './types'

/** Reads the caller's wallpaper, the price, and this month's remaining changes. */
export async function getWallpaper(): Promise<ApiResponse<WallpaperStatus>> {
  const response = await api.get<ApiResponse<WallpaperStatus>>(
    '/api/user/wallpaper'
  )
  return requireServerSuccess(response.data)
}

/** Buys one change: charges the caller's wallet and records the new wallpaper. */
export async function updateWallpaper(
  wallpaperUrl: string
): Promise<ApiResponse<WallpaperStatus>> {
  const response = await api.put<ApiResponse<WallpaperStatus>>(
    '/api/user/wallpaper',
    { wallpaper_url: wallpaperUrl }
  )
  return requireServerSuccess(response.data)
}

/** Clears the caller's wallpaper. Free, and does not return a change. */
export async function clearWallpaper(): Promise<ApiResponse> {
  const response = await api.delete<ApiResponse>('/api/user/wallpaper')
  return requireServerSuccess(response.data)
}
