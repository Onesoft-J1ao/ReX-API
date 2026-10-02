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
  pickDominantAccent,
  readableForegroundForHsl,
  type WallpaperAccent,
} from '../dominant-color'

type Rgba = [number, number, number, number]

function rgba(colors: Rgba[]): Uint8ClampedArray {
  const pixels = new Uint8ClampedArray(colors.length * 4)
  colors.forEach(([r, g, b, a], index) => {
    pixels[index * 4] = r
    pixels[index * 4 + 1] = g
    pixels[index * 4 + 2] = b
    pixels[index * 4 + 3] = a
  })
  return pixels
}

function repeat(color: Rgba, times: number): Rgba[] {
  return Array.from({ length: times }, () => color)
}

function accentOf(accent: WallpaperAccent | null): WallpaperAccent {
  if (!accent) throw new Error('expected the image to yield an accent')
  return accent
}

function hueOf(accent: WallpaperAccent | null): number {
  const match = /^hsl\(([\d.]+)/.exec(accentOf(accent).accent)
  if (!match) throw new Error('accent is not an hsl() colour')
  return Number(match[1])
}

function lightnessOf(accent: WallpaperAccent | null): number {
  const match = /([\d.]+)%\)$/.exec(accentOf(accent).accent)
  if (!match) throw new Error('accent carries no lightness')
  return Number(match[1])
}

describe('pickDominantAccent', () => {
  it('takes the hue of a uniformly coloured image', () => {
    const accent = accentOf(
      pickDominantAccent(rgba(repeat([30, 90, 220, 255], 64)))
    )
    expect(Math.abs(hueOf(accent) - 221)).toBeLessThan(6)
    expect(accent.accent).toMatch(/^hsl\([\d.]+ [\d.]+% [\d.]+%\)$/)
  })

  it('prefers a vivid minority over a dull majority', () => {
    // A grey wash must not supply the accent just because it covers more pixels.
    const accent = pickDominantAccent(
      rgba([
        ...repeat([128, 128, 128, 255], 200),
        ...repeat([240, 120, 20, 255], 80),
      ])
    )
    expect(Math.abs(hueOf(accent) - 27)).toBeLessThan(8)
  })

  it('returns nothing when the image has no colour to take', () => {
    expect(pickDominantAccent(rgba(repeat([128, 128, 128, 255], 16)))).toBeNull()
    expect(pickDominantAccent(rgba(repeat([0, 0, 0, 255], 16)))).toBeNull()
    expect(pickDominantAccent(rgba(repeat([255, 255, 255, 255], 16)))).toBeNull()
  })

  it('ignores transparent pixels', () => {
    expect(pickDominantAccent(rgba(repeat([30, 90, 220, 0], 64)))).toBeNull()
  })

  it('returns nothing for an empty buffer', () => {
    expect(pickDominantAccent(new Uint8ClampedArray(0))).toBeNull()
  })

  it('keeps a saturated blue, which a brightness-weighted test would drop', () => {
    // Relative luminance scores blue at 0.07, so filtering on brightness would
    // throw a deep blue wallpaper away; chroma is what decides instead.
    expect(
      Math.abs(hueOf(pickDominantAccent(rgba(repeat([0, 0, 255, 255], 16)))) - 240)
    ).toBeLessThan(4)
  })

  it('keeps the accent inside a legible lightness band', () => {
    // A near-black blue would make an unreadable highlight; the band lifts it.
    const lightness = lightnessOf(
      pickDominantAccent(rgba(repeat([0, 0, 40, 255], 16)))
    )
    expect(lightness).toBeGreaterThanOrEqual(42)
    expect(lightness).toBeLessThanOrEqual(62)
  })
})

describe('readableForegroundForHsl', () => {
  it('puts dark text on a light accent', () => {
    expect(readableForegroundForHsl(60, 0.9, 0.62)).toBe('#0b0b0c')
  })

  it('puts white text on a dark accent', () => {
    expect(readableForegroundForHsl(250, 0.9, 0.42)).toBe('#ffffff')
  })
})
