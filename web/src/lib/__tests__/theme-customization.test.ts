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
import { describe, expect, it } from 'vitest'

import {
  DEFAULT_WALLPAPER_FRAMING,
  isDefaultWallpaperFraming,
  isVideoWallpaperUrl,
  normalizeWallpaperFraming,
  parseWallpaperFraming,
  parseWallpaperVisibility,
  WALLPAPER_VISIBILITY_DEFAULT,
  WALLPAPER_VISIBILITY_MAX,
  WALLPAPER_VISIBILITY_MIN,
  serializeWallpaperFraming,
  wallpaperSurfaceOpacity,
  WALLPAPER_ZOOM_MAX,
  WALLPAPER_ZOOM_MIN,
  wallpaperBackgroundSize,
} from '../theme-customization'

describe('parseWallpaperFraming', () => {
  it('reads the three numbers the cookie holds', () => {
    expect(parseWallpaperFraming('1.40,25,75')).toEqual({
      zoom: 1.4,
      focusX: 25,
      focusY: 75,
      fit: 'cover',
    })
  })

  it('falls back to the default for anything unreadable', () => {
    for (const raw of ['', null, undefined, '2,10', 'a,b,c', '1.5,2,3,4,5']) {
      expect(parseWallpaperFraming(raw)).toEqual(DEFAULT_WALLPAPER_FRAMING)
    }
  })

  it('clamps values a hand-edited cookie could carry', () => {
    expect(parseWallpaperFraming('99,150,-20')).toEqual({
      zoom: WALLPAPER_ZOOM_MAX,
      focusX: 100,
      focusY: 0,
      fit: 'cover',
    })
  })
})

describe('serializeWallpaperFraming', () => {
  it('round-trips through the parser', () => {
    const framing = { zoom: 2.5, focusX: 12, focusY: 88, fit: 'cover' as const }
    expect(parseWallpaperFraming(serializeWallpaperFraming(framing))).toEqual(
      framing
    )
  })

  it('writes a zoom the cookie can hold to two decimals', () => {
    expect(
      serializeWallpaperFraming({
        zoom: 1.234567,
        focusX: 50.4,
        focusY: 49.6,
        fit: 'contain',
      })
    ).toBe('1.23,50,50,contain')
  })
})

describe('normalizeWallpaperFraming', () => {
  it('replaces a non-finite axis with the default rather than NaN', () => {
    expect(
      normalizeWallpaperFraming({
        zoom: Number.NaN,
        focusX: Number.POSITIVE_INFINITY,
        focusY: 40,
        fit: 'sideways' as never,
      })
    ).toEqual({
      zoom: WALLPAPER_ZOOM_MIN,
      focusX: 50,
      focusY: 40,
      fit: 'cover',
    })
  })
})

describe('isDefaultWallpaperFraming', () => {
  it('is true only for the untouched crop', () => {
    expect(isDefaultWallpaperFraming(DEFAULT_WALLPAPER_FRAMING)).toBe(true)
    expect(
      isDefaultWallpaperFraming({
        zoom: 1.05,
        focusX: 50,
        focusY: 50,
        fit: 'cover',
      })
    ).toBe(false)
    expect(
      isDefaultWallpaperFraming({
        zoom: 1,
        focusX: 50,
        focusY: 49,
        fit: 'cover',
      })
    ).toBe(false)
    expect(
      isDefaultWallpaperFraming({
        zoom: 1,
        focusX: 50,
        focusY: 50,
        fit: 'contain',
      })
    ).toBe(false)
  })
})

describe('wallpaperBackgroundSize', () => {
  it('leaves the untouched crop on plain cover', () => {
    expect(wallpaperBackgroundSize(1, 1.6, 1.78)).toBe('cover')
  })

  it('scales the height for an image wider than the viewport', () => {
    // `cover` fits the height here, so that is the axis to multiply.
    expect(wallpaperBackgroundSize(2, 2.4, 1.78)).toBe('auto calc(100% * 2)')
  })

  it('scales the width for an image taller than the viewport', () => {
    expect(wallpaperBackgroundSize(2, 1, 1.78)).toBe('calc(100% * 2) auto')
  })

  it('stays on cover when the image has not been measured', () => {
    expect(wallpaperBackgroundSize(2, null, 1.78)).toBe('cover')
    expect(wallpaperBackgroundSize(2, 1.6, null)).toBe('cover')
    expect(wallpaperBackgroundSize(2, 0, 1.78)).toBe('cover')
  })

  it('clamps the zoom it writes', () => {
    expect(wallpaperBackgroundSize(9, 1, 1.78)).toBe(
      `calc(100% * ${WALLPAPER_ZOOM_MAX}) auto`
    )
  })
})

describe('the fit axis', () => {
  it('is read from the fourth cookie field when it is there', () => {
    expect(parseWallpaperFraming('1.00,50,50,contain').fit).toBe('contain')
    expect(parseWallpaperFraming('1.00,50,50,sideways').fit).toBe('cover')
  })

  it('scales the long axis for contain, where cover scales the short one', () => {
    // Wider than the viewport: cover fills the height, contain fits the width.
    expect(wallpaperBackgroundSize(2, 2.4, 1.78, 'cover')).toBe(
      'auto calc(100% * 2)'
    )
    expect(wallpaperBackgroundSize(2, 2.4, 1.78, 'contain')).toBe(
      'calc(100% * 2) auto'
    )
  })

  it('stays on the plain keyword at 1x and when unmeasured', () => {
    expect(wallpaperBackgroundSize(1, 2.4, 1.78, 'contain')).toBe('contain')
    expect(wallpaperBackgroundSize(2, null, 1.78, 'contain')).toBe('contain')
  })
})

describe('wallpaper visibility', () => {
  it('defaults, clamps and inverts into the surface opacity', () => {
    expect(parseWallpaperVisibility(null)).toBe(WALLPAPER_VISIBILITY_DEFAULT)
    expect(parseWallpaperVisibility('nonsense')).toBe(
      WALLPAPER_VISIBILITY_DEFAULT
    )
    expect(parseWallpaperVisibility('500')).toBe(WALLPAPER_VISIBILITY_MAX)
    expect(parseWallpaperVisibility('-20')).toBe(WALLPAPER_VISIBILITY_MIN)
    // Inverted, and clamped first: the slider's ceiling is what keeps a busy
    // picture from washing the panels out entirely.
    expect(wallpaperSurfaceOpacity(0)).toBe(100)
    expect(wallpaperSurfaceOpacity(WALLPAPER_VISIBILITY_MAX)).toBe(
      100 - WALLPAPER_VISIBILITY_MAX
    )
    expect(wallpaperSurfaceOpacity(100)).toBe(100 - WALLPAPER_VISIBILITY_MAX)
  })
})

describe('isVideoWallpaperUrl', () => {
  it('recognises the containers a wallpaper may point at', () => {
    expect(isVideoWallpaperUrl('https://cdn.test/loop.mp4')).toBe(true)
    expect(isVideoWallpaperUrl('https://cdn.test/loop.webm')).toBe(true)
    expect(isVideoWallpaperUrl('https://cdn.test/LOOP.MP4')).toBe(true)
  })

  it('ignores the query and fragment a CDN address carries', () => {
    expect(isVideoWallpaperUrl('https://cdn.test/loop.mp4?token=abc')).toBe(true)
    expect(isVideoWallpaperUrl('https://cdn.test/loop.webm#t=3')).toBe(true)
  })

  it('does not mistake a picture for a video', () => {
    expect(isVideoWallpaperUrl('https://cdn.test/wall.jpg')).toBe(false)
    expect(isVideoWallpaperUrl('https://cdn.test/wall')).toBe(false)
    // What a Steam Workshop item resolves to: a hash directory, not a file.
    expect(
      isVideoWallpaperUrl(
        'https://images.steamusercontent.com/ugc/11930026390145821500/B2E419DA46294C6C6A28192E3C1E75B15230D731/'
      )
    ).toBe(false)
  })

  it('is false for anything unusable', () => {
    expect(isVideoWallpaperUrl('')).toBe(false)
    expect(isVideoWallpaperUrl('not a url')).toBe(false)
    expect(isVideoWallpaperUrl('/local.mp4')).toBe(false)
  })
})
