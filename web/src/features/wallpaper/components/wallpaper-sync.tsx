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
import { useEffect } from 'react'

import { useThemeCustomization } from '@/context/theme-customization-provider'

import { useWallpaper } from '../hooks/use-wallpaper'
import { WallpaperFramingOverlay } from './wallpaper-framing-overlay'

/**
 * Hydrates the signed-in user's paid wallpaper into the theme provider, which
 * is what actually paints it. Mounted once by the console layout and renders
 * nothing else; the public pages never mount it, so they never fetch.
 *
 * The drag-to-frame overlay is rendered from here rather than from the provider
 * so the provider stays free of console UI (and of an import cycle).
 */
export function WallpaperSync() {
  const { setWallpaper, wallpaperFramingMode, wallpaperUrl } =
    useThemeCustomization()
  const { data } = useWallpaper()
  const paidWallpaperUrl = data?.wallpaper_url ?? ''

  useEffect(() => {
    setWallpaper(paidWallpaperUrl)
  }, [paidWallpaperUrl, setWallpaper])

  return wallpaperFramingMode && wallpaperUrl !== '' ? (
    <WallpaperFramingOverlay />
  ) : null
}
