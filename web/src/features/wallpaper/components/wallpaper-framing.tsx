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
import { Move, RotateCcw } from 'lucide-react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Slider } from '@/components/ui/slider'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import {
  isDefaultWallpaperFraming,
  isVideoWallpaperUrl,
  normalizeWallpaperFraming,
  WALLPAPER_ZOOM_MAX,
  WALLPAPER_ZOOM_MIN,
} from '@/lib/theme-customization'
import { cn } from '@/lib/utils'

type FramingSliderProps = {
  label: string
  value: number
  min: number
  max: number
  step: number
  format: (value: number) => string
  onChange: (value: number) => void
}

function FramingSlider(props: FramingSliderProps) {
  return (
    <div className='flex items-center gap-3'>
      <span className='text-muted-foreground w-14 shrink-0 text-xs'>
        {props.label}
      </span>
      <Slider
        value={props.value}
        min={props.min}
        max={props.max}
        step={props.step}
        aria-label={props.label}
        onValueChange={(next) => {
          const value = Array.isArray(next) ? next[0] : next
          if (typeof value === 'number') props.onChange(value)
        }}
        className='flex-1'
      />
      <span className='text-muted-foreground w-12 shrink-0 text-right font-mono text-xs tabular-nums'>
        {props.format(props.value)}
      </span>
    </div>
  )
}

/**
 * The crop editor for the wallpaper.
 *
 * "Cropping" here is framing rather than a new bitmap: the stored value is a
 * URL, so what the user chooses is which point of the picture sits at the
 * centre of the screen and how far in the zoom is. That makes it free to adjust
 * and per-browser, so it writes to the theme provider instead of spending one of
 * the month's paid wallpaper changes.
 */
export function WallpaperFraming(props: { url: string }) {
  const { t } = useTranslation()
  const { wallpaperFraming, setWallpaperFraming, resetWallpaperFraming } =
    useThemeCustomization()
  const surfaceRef = useRef<HTMLDivElement>(null)
  const dragStart = useRef<{
    pointerX: number
    pointerY: number
    focusX: number
    focusY: number
  } | null>(null)

  const { zoom, focusX, focusY } = wallpaperFraming
  const isDefault = isDefaultWallpaperFraming(wallpaperFraming)

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
    const surface = surfaceRef.current
    if (!start || !surface) return
    const { width, height } = surface.getBoundingClientRect()
    if (width === 0 || height === 0) return
    // The picture follows the pointer, so dragging right reveals what is to its
    // left: the focus point moves the other way.
    setWallpaperFraming(
      normalizeWallpaperFraming({
        zoom,
        focusX: start.focusX - ((event.clientX - start.pointerX) / width) * 100,
        focusY:
          start.focusY - ((event.clientY - start.pointerY) / height) * 100,
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
    <div className='mb-3'>
      <div
        ref={surfaceRef}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        // touch-none keeps a drag from scrolling the drawer instead of framing.
        className='ring-border relative mb-1 h-28 w-full cursor-grab touch-none overflow-hidden rounded-md ring-[1px] active:cursor-grabbing'
      >
        {isVideoWallpaperUrl(props.url) ? (
          // The crop has to work for a video too, so the preview plays it just
          // as the wallpaper layer will.
          <video
            src={props.url}
            aria-hidden='true'
            muted
            loop
            autoPlay
            playsInline
            className='pointer-events-none h-full w-full select-none object-cover'
            style={{
              objectPosition: `${focusX}% ${focusY}%`,
              transform: `scale(${zoom})`,
              transformOrigin: `${focusX}% ${focusY}%`,
            }}
          />
        ) : (
          <img
            src={props.url}
            alt=''
            aria-hidden='true'
            draggable={false}
            referrerPolicy='no-referrer'
            className='h-full w-full select-none object-cover'
            style={{
              objectPosition: `${focusX}% ${focusY}%`,
              transform: `scale(${zoom})`,
              // Scaling about the focus point keeps the point the user picked
              // exactly where object-position put it.
              transformOrigin: `${focusX}% ${focusY}%`,
            }}
          />
        )}
        <span className='text-muted-foreground/80 pointer-events-none absolute right-2 bottom-2 flex items-center gap-1 rounded-full bg-black/35 px-2 py-0.5 text-[11px] text-white/85 backdrop-blur-sm'>
          <Move className='size-3' aria-hidden='true' />
          {t('Drag to frame')}
        </span>
      </div>

      <div className='mb-2 space-y-2'>
        <FramingSlider
          label={t('Zoom')}
          value={zoom}
          min={WALLPAPER_ZOOM_MIN}
          max={WALLPAPER_ZOOM_MAX}
          step={0.05}
          format={(value) => `${value.toFixed(2)}x`}
          onChange={(value) =>
            setWallpaperFraming({ zoom: value, focusX, focusY })
          }
        />
        <FramingSlider
          label={t('Horizontal')}
          value={focusX}
          min={0}
          max={100}
          step={1}
          format={(value) => `${Math.round(value)}%`}
          onChange={(value) =>
            setWallpaperFraming({ zoom, focusX: value, focusY })
          }
        />
        <FramingSlider
          label={t('Vertical')}
          value={focusY}
          min={0}
          max={100}
          step={1}
          format={(value) => `${Math.round(value)}%`}
          onChange={(value) =>
            setWallpaperFraming({ zoom, focusX, focusY: value })
          }
        />
      </div>

      <Button
        size='sm'
        variant='ghost'
        className={cn('h-7 px-2 text-xs', isDefault && 'invisible')}
        onClick={resetWallpaperFraming}
      >
        <RotateCcw className='mr-1.5 size-3' aria-hidden='true' />
        {t('Reset framing')}
      </Button>
    </div>
  )
}
