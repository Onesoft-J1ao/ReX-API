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
import { Check, Move, RotateCcw } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import {
  normalizeWallpaperFraming,
  THEME_WALLPAPER_FRAMING_SLOT,
  WALLPAPER_ZOOM_MAX,
  WALLPAPER_ZOOM_MIN,
} from '@/lib/theme-customization'
import { cn } from '@/lib/utils'

import { FramingSlider } from './framing-slider'

/**
 * The whole page as a drag surface for placing the wallpaper.
 *
 * The preview in the settings is only a thumbnail, so a picture that does not
 * sit right in it can look wrong everywhere else. This overlay lets the picture
 * be pushed around at the size it is actually shown, with the sliders kept for
 * the adjustments a drag cannot make precisely.
 */
export function WallpaperFramingOverlay() {
  const { t } = useTranslation()
  const { wallpaperFraming, setWallpaperFraming, resetWallpaperFraming, setWallpaperFramingMode } =
    useThemeCustomization()
  const dragStart = useRef<{
    pointerX: number
    pointerY: number
    focusX: number
    focusY: number
  } | null>(null)

  const { zoom, focusX, focusY, fit } = wallpaperFraming
  const isDefaultFraming =
    zoom === WALLPAPER_ZOOM_MIN && focusX === 50 && focusY === 50 && fit === 'cover'

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setWallpaperFramingMode(false)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [setWallpaperFramingMode])

  const handlePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    dragStart.current = {
      pointerX: event.clientX,
      pointerY: event.clientY,
      focusX,
      focusY,
    }
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  const handlePointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    const start = dragStart.current
    if (!start) return
    if (window.innerWidth === 0 || window.innerHeight === 0) return
    // The picture follows the pointer, so dragging right reveals what is to its
    // left: the focus point moves the other way.
    setWallpaperFraming(
      normalizeWallpaperFraming({
        ...wallpaperFraming,
        focusX:
          start.focusX -
          ((event.clientX - start.pointerX) / window.innerWidth) * 100,
        focusY:
          start.focusY -
          ((event.clientY - start.pointerY) / window.innerHeight) * 100,
      })
    )
  }

  const endDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    dragStart.current = null
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
  }

  return (
    <div
      data-slot={THEME_WALLPAPER_FRAMING_SLOT}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      className='fixed inset-0 z-[90] cursor-grab touch-none select-none active:cursor-grabbing'
    >
      <div className='pointer-events-none absolute inset-x-0 top-0 flex justify-center pt-4'>
        <span className='flex items-center gap-2 rounded-full bg-black/55 px-3 py-1.5 text-xs text-white/90 backdrop-blur-sm'>
          <Move className='size-3.5' aria-hidden='true' />
          {t('Drag anywhere to reposition the wallpaper')}
        </span>
      </div>

      <div
        // The bar is the only interactive part; the rest of the overlay is the
        // drag surface.
        className='absolute inset-x-0 bottom-0 border-t border-white/10 bg-black/65 px-4 py-3 backdrop-blur-md'
      >
        <div className='mx-auto flex max-w-3xl flex-col gap-2'>
          <FramingSlider
            label={t('Zoom')}
            value={zoom}
            min={WALLPAPER_ZOOM_MIN}
            max={WALLPAPER_ZOOM_MAX}
            step={0.05}
            tone='overlay'
            format={(value) => `${value.toFixed(2)}x`}
            onChange={(value) =>
              setWallpaperFraming({ ...wallpaperFraming, zoom: value })
            }
          />
          <FramingSlider
            label={t('Horizontal')}
            value={focusX}
            min={0}
            max={100}
            step={1}
            tone='overlay'
            format={(value) => `${Math.round(value)}%`}
            onChange={(value) =>
              setWallpaperFraming({ ...wallpaperFraming, focusX: value })
            }
          />
          <FramingSlider
            label={t('Vertical')}
            value={focusY}
            min={0}
            max={100}
            step={1}
            tone='overlay'
            format={(value) => `${Math.round(value)}%`}
            onChange={(value) =>
              setWallpaperFraming({ ...wallpaperFraming, focusY: value })
            }
          />
          <div className='flex flex-wrap items-center gap-2 pt-1'>
            <Button
              size='sm'
              variant='outline'
              className={cn(
                'h-7 px-2 text-xs',
                fit === 'cover'
                  ? 'bg-white/15 text-white'
                  : 'text-white/70 hover:text-white'
              )}
              onClick={() =>
                setWallpaperFraming({ ...wallpaperFraming, fit: 'cover' })
              }
            >
              {t('Fill the screen')}
            </Button>
            <Button
              size='sm'
              variant='outline'
              className={cn(
                'h-7 px-2 text-xs',
                fit === 'contain'
                  ? 'bg-white/15 text-white'
                  : 'text-white/70 hover:text-white'
              )}
              onClick={() =>
                setWallpaperFraming({ ...wallpaperFraming, fit: 'contain' })
              }
            >
              {t('Show the whole picture')}
            </Button>
            <span className='flex-1' />
            <Button
              size='sm'
              variant='outline'
              className={cn(
                'h-7 px-2 text-xs text-white/70 hover:text-white',
                isDefaultFraming && 'invisible'
              )}
              onClick={resetWallpaperFraming}
            >
              <RotateCcw className='mr-1.5 size-3' aria-hidden='true' />
              {t('Reset framing')}
            </Button>
            <Button
              size='sm'
              className='h-7 px-3 text-xs'
              onClick={() => setWallpaperFramingMode(false)}
            >
              <Check className='mr-1.5 size-3' aria-hidden='true' />
              {t('Done')}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
