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
 * Derives the accent colour from a wallpaper image.
 *
 * The wallpaper is somebody else's URL, which decides whether this works at all:
 * reading pixels needs the image to arrive with CORS headers, and a host that
 * does not send them taints the canvas instead. Every failure path returns
 * `null` and the caller keeps the preset's own accent, so a wallpaper that
 * cannot be sampled still renders normally.
 */

export type WallpaperAccent = {
  /** Ready to drop into a CSS custom property. */
  accent: string
  /** Text colour that stays readable on top of `accent`. */
  foreground: string
}

/** Sampling resolution. A wallpaper's palette survives this shrink. */
const SAMPLE_SIZE = 48

/** Pixels below this alpha are treated as not part of the picture. */
const MIN_ALPHA = 128

/**
 * How far apart the channels have to be before a pixel counts as coloured.
 *
 * Chroma alone decides this, deliberately: a brightness test would have to be
 * weighted (relative luminance gives blue 0.07), and a saturated blue wallpaper
 * would then be thrown away as "too dark". A near-black or near-white pixel is
 * already excluded by its own low chroma, and the lightness band below lifts
 * whatever survives into something usable.
 */
const MIN_CHROMA = 24

/** The accent is forced into this band so it stays legible as a highlight. */
const ACCENT_MIN_SATURATION = 0.45
const ACCENT_MAX_SATURATION = 0.92
const ACCENT_MIN_LIGHTNESS = 0.42
const ACCENT_MAX_LIGHTNESS = 0.62

function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, value))
}

function relativeLuminance(r: number, g: number, b: number): number {
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function toHsl(
  r: number,
  g: number,
  b: number
): { h: number; s: number; l: number } {
  const rn = r / 255
  const gn = g / 255
  const bn = b / 255
  const max = Math.max(rn, gn, bn)
  const min = Math.min(rn, gn, bn)
  const l = (max + min) / 2
  const delta = max - min
  if (delta === 0) return { h: 0, s: 0, l }

  const s = delta / (1 - Math.abs(2 * l - 1))
  let h: number
  if (max === rn) {
    h = ((gn - bn) / delta) % 6
  } else if (max === gn) {
    h = (bn - rn) / delta + 2
  } else {
    h = (rn - gn) / delta + 4
  }
  return { h: (h * 60 + 360) % 360, s, l }
}

/**
 * Picks the dominant vivid colour out of RGBA pixel data.
 *
 * Pure, so the palette maths is testable without a canvas. Returns `null` when
 * no pixel can carry an accent: a fully transparent image, or one that is only
 * black, white and grey has no colour to take, and inventing a hue for it would
 * be worse than leaving the preset's accent alone.
 */
export function pickDominantAccent(
  pixels: Uint8ClampedArray
): WallpaperAccent | null {
  // 4 bits per channel: 4096 buckets, coarse enough to group a gradient's
  // neighbouring shades and fine enough to keep two hues apart.
  const buckets = new Map<
    number,
    { count: number; r: number; g: number; b: number }
  >()

  const total = Math.floor(pixels.length / 4)
  for (let i = 0; i < total; i += 1) {
    const offset = i * 4
    if (pixels[offset + 3] < MIN_ALPHA) continue
    const r = pixels[offset]
    const g = pixels[offset + 1]
    const b = pixels[offset + 2]

    if (Math.max(r, g, b) - Math.min(r, g, b) < MIN_CHROMA) continue

    const key = ((r >> 4) << 8) | ((g >> 4) << 4) | (b >> 4)
    const entry = buckets.get(key)
    if (entry) {
      entry.count += 1
      entry.r += r
      entry.g += g
      entry.b += b
    } else {
      buckets.set(key, { count: 1, r, g, b })
    }
  }

  let winner: { count: number; r: number; g: number; b: number } | null = null
  for (const entry of buckets.values()) {
    if (!winner || entry.count > winner.count) winner = entry
  }
  if (!winner) return null

  const { h, s, l } = toHsl(
    winner.r / winner.count,
    winner.g / winner.count,
    winner.b / winner.count
  )
  const accentSaturation = clamp(
    s,
    ACCENT_MIN_SATURATION,
    ACCENT_MAX_SATURATION
  )
  const accentLightness = clamp(l, ACCENT_MIN_LIGHTNESS, ACCENT_MAX_LIGHTNESS)

  const accent = `hsl(${h.toFixed(1)} ${(accentSaturation * 100).toFixed(1)}% ${(accentLightness * 100).toFixed(1)}%)`
  return {
    accent,
    // The forced lightness band spans mid tones, so the readable foreground has
    // to be measured rather than assumed from the source pixel.
    foreground: readableForegroundForHsl(h, accentSaturation, accentLightness),
  }
}

/** Black or white, whichever contrasts better with the accent. */
export function readableForegroundForHsl(
  h: number,
  s: number,
  l: number
): string {
  const rgb = hslToRgb(h, s, l)
  return relativeLuminance(rgb.r, rgb.g, rgb.b) > 150 ? '#0b0b0c' : '#ffffff'
}

function hslToRgb(
  h: number,
  s: number,
  l: number
): { r: number; g: number; b: number } {
  const c = (1 - Math.abs(2 * l - 1)) * s
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = l - c / 2
  let rgb: [number, number, number]
  if (h < 60) rgb = [c, x, 0]
  else if (h < 120) rgb = [x, c, 0]
  else if (h < 180) rgb = [0, c, x]
  else if (h < 240) rgb = [0, x, c]
  else if (h < 300) rgb = [x, 0, c]
  else rgb = [c, 0, x]
  return {
    r: (rgb[0] + m) * 255,
    g: (rgb[1] + m) * 255,
    b: (rgb[2] + m) * 255,
  }
}

/**
 * Loads the wallpaper and reports its accent, or `null` when it cannot be read
 * — a failed load, or a canvas tainted by a host that sends no CORS headers.
 */
export async function sampleWallpaperAccent(
  url: string
): Promise<WallpaperAccent | null> {
  if (typeof document === 'undefined' || url === '') return null

  const image = new Image()
  // Without this the canvas is tainted for every cross-origin wallpaper and
  // pixel access throws.
  image.crossOrigin = 'anonymous'
  image.referrerPolicy = 'no-referrer'
  image.decoding = 'async'

  try {
    await new Promise<void>((resolve, reject) => {
      image.addEventListener('load', () => resolve(), { once: true })
      image.addEventListener(
        'error',
        () => reject(new Error('wallpaper image failed to load')),
        { once: true }
      )
      image.src = url
    })
  } catch {
    return null
  }

  if (!image.naturalWidth || !image.naturalHeight) return null

  const canvas = document.createElement('canvas')
  canvas.width = SAMPLE_SIZE
  canvas.height = SAMPLE_SIZE
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context) return null

  try {
    context.drawImage(image, 0, 0, SAMPLE_SIZE, SAMPLE_SIZE)
    const { data } = context.getImageData(0, 0, SAMPLE_SIZE, SAMPLE_SIZE)
    return pickDominantAccent(data)
  } catch {
    // SecurityError: the host does not allow cross-origin reads.
    return null
  }
}
