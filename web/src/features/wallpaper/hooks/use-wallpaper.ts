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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { clearWallpaper, getWallpaper, resolveWallpaperSource, updateWallpaper } from '../api'

/**
 * One query key covers everything the console needs to render the wallpaper
 * setting, so a paid change refreshes the allowance and the preview together.
 */
export const wallpaperQueryKey = ['wallpaper', 'status'] as const

/**
 * The wallpaper settings read. `errorToast: false` because the console shows
 * the wallpaper section only when this succeeds — a toast for a background
 * read would be noise the user cannot act on.
 */
export function useWallpaper(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: wallpaperQueryKey,
    queryFn: getWallpaper,
    select: (response) => response.data,
    enabled: options?.enabled ?? true,
    meta: { errorToast: false },
  })
}

/** Buying a change returns the refreshed status, so no refetch is needed. */
export function useUpdateWallpaper() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: updateWallpaper,
    meta: { errorToast: false },
    onSuccess: (response) => {
      queryClient.setQueryData(wallpaperQueryKey, response)
    },
  })
}

/** Clearing is free and keeps the allowance, so only the cached read changes. */
export function useClearWallpaper() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: clearWallpaper,
    meta: { errorToast: false },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: wallpaperQueryKey })
    },
  })
}

/**
 * Resolving a Wallpaper Engine link changes nothing on the server, so there is
 * no cache to touch: the caller only needs the image address it returns.
 */
export function useResolveWallpaperSource() {
  return useMutation({
    mutationFn: resolveWallpaperSource,
    meta: { errorToast: false },
  })
}
