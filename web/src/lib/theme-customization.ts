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
 * Theme customization constants and types.
 *
 * Lives in `lib/` (not `context/`) so it can be imported alongside the
 * provider without breaking React Fast Refresh boundaries.
 */

export const THEME_PRESETS = [
  {
    value: 'default',
    name: 'Default',
    swatches: ['oklch(0.72 0.18 250)', 'oklch(0.7 0.12 280)'],
  },
  {
    // Inspired by Anthropic's official brand language: warm cream canvas
    // (#faf9f5) paired with clay/coral (#d97757) as the single accent.
    // Swatches preview the canvas → accent gradient that defines the system.
    value: 'anthropic',
    name: 'Anthropic',
    swatches: ['oklch(0.984 0.005 95)', 'oklch(0.685 0.142 38)'],
  },
  {
    value: 'simple-large',
    name: 'Simple Large-font',
    swatches: ['oklch(0.15 0 0)', 'oklch(0.99 0 0)'],
  },
  {
    value: 'underground',
    name: 'Underground',
    swatches: ['oklch(0.5315 0.0694 156.19)', 'oklch(0.5748 0.0862 336.52)'],
  },
  {
    value: 'rose-garden',
    name: 'Rose Garden',
    swatches: ['oklch(0.5827 0.2418 12.23)', 'oklch(0.8131 0.1129 5.67)'],
  },
  {
    value: 'lake-view',
    name: 'Lake View',
    swatches: ['oklch(0.765 0.177 163.22)', 'oklch(0.551 0.0899 200.52)'],
  },
  {
    value: 'sunset-glow',
    name: 'Sunset Glow',
    swatches: ['oklch(0.5591 0.1882 25.33)', 'oklch(0.7938 0.1248 42.42)'],
  },
  {
    value: 'forest-whisper',
    name: 'Forest Whisper',
    swatches: ['oklch(0.5276 0.1072 182.22)', 'oklch(0.5236 0.0505 250.18)'],
  },
  {
    value: 'ocean-breeze',
    name: 'Ocean Breeze',
    swatches: ['oklch(0.5461 0.2152 262.88)', 'oklch(0.5854 0.2041 277.12)'],
  },
  {
    value: 'lavender-dream',
    name: 'Lavender Dream',
    swatches: ['oklch(0.5709 0.1808 306.89)', 'oklch(0.811 0.0589 201.14)'],
  },
  {
    // Liquid glass: a cool, high-chroma base whose surfaces are meant to be
    // read through rather than on. Picking it turns the glass layer on by
    // itself (see `resolveLiquidMode`), so the preset and the wallpaper both
    // reach the same look.
    value: 'liquid',
    name: 'Liquid Glass',
    swatches: ['oklch(0.62 0.19 258)', 'oklch(0.78 0.14 196)'],
  },
] as const

export type ThemePreset = (typeof THEME_PRESETS)[number]['value']
export type ThemeRadius = 'default' | 'none' | 'sm' | 'md' | 'lg' | 'xl'
export type ThemeScale = 'default' | 'sm' | 'lg' | 'xl'
export type ContentLayout = 'full' | 'centered'

/**
 * Font axis for the theme.
 *
 * - `default` — resolve at runtime from the active preset
 *   (see `PRESET_DEFAULT_FONT`). The shipped `default` and `anthropic`
 *   presets resolve to serif; other named color presets fall back to
 *   sans unless they list a different choice. Mirrors how
 *   `radius: 'default'` defers to a per-preset hint.
 * - `sans` — humanist sans (Public Sans), the project's UI fallback.
 * - `serif` — editorial serif (Lora + CJK fallbacks), the project's
 *   "soul" typography. Inherits across the whole UI; monospace contexts
 *   keep their own family via Tailwind preflight and `.font-mono`.
 */
export type ThemeFont = 'default' | 'sans' | 'serif'

/**
 * The resolved (non-`default`) font value applied to the DOM. The provider
 * always sets `data-theme-font` to one of these concrete values so CSS only
 * needs simple attribute selectors (no `:not()` gymnastics, no per-preset
 * font branches).
 */
export type ResolvedThemeFont = Exclude<ThemeFont, 'default'>

export type ThemeCustomization = {
  preset: ThemePreset
  font: ThemeFont
  radius: ThemeRadius
  scale: ThemeScale
  contentLayout: ContentLayout
}

export const DEFAULT_THEME_CUSTOMIZATION: ThemeCustomization = {
  preset: 'default',
  font: 'default',
  radius: 'default',
  scale: 'default',
  contentLayout: 'full',
}

export const THEME_PRESET_VALUES = new Set(
  THEME_PRESETS.map((p) => p.value)
) as ReadonlySet<ThemePreset>

export const THEME_FONT_VALUES: ReadonlySet<ThemeFont> = new Set([
  'default',
  'sans',
  'serif',
])

export const THEME_RADIUS_VALUES: ReadonlySet<ThemeRadius> = new Set([
  'default',
  'none',
  'sm',
  'md',
  'lg',
  'xl',
])

export const THEME_SCALE_VALUES: ReadonlySet<ThemeScale> = new Set([
  'default',
  'sm',
  'lg',
  'xl',
])

export const CONTENT_LAYOUT_VALUES: ReadonlySet<ContentLayout> = new Set([
  'full',
  'centered',
])

export const THEME_COOKIE_KEYS = {
  preset: 'theme_preset',
  font: 'theme_font',
  radius: 'theme_radius',
  scale: 'theme_scale',
  contentLayout: 'theme_content_layout',
} as const

/**
 * The wallpaper axis is the one appearance setting that is not a cookie: it is
 * bought, so the server owns it and the console hydrates it from the user's
 * own record. It still flows through this provider — the single place that
 * writes appearance attributes onto `<body>` — so the wallpaper is painted by
 * the same cascade as the presets (see `styles/theme-presets.css`).
 */
export const THEME_WALLPAPER_ATTRIBUTE = 'data-theme-wallpaper'
export const THEME_WALLPAPER_VALUE = 'custom'
export const THEME_WALLPAPER_VARIABLE = '--wallpaper-image'

/**
 * The liquid (glass) surface layer.
 *
 * Liquid is a surface treatment, not a palette: it makes cards, popovers, the
 * sidebar and dialogs translucent with a blurred backdrop and a light top
 * edge, so whatever is behind them reads through. That is why it is a separate
 * attribute rather than another color preset — it composes with the user's
 * chosen colors instead of replacing them.
 *
 * `resolveLiquidMode` turns it on in two cases: the user picked the Liquid
 * Glass preset, or the user has a wallpaper. The second is the point of the
 * mode — a wallpaper is the one background worth showing through the console —
 * and it is derived at render time rather than written to a cookie, so removing
 * the wallpaper restores the user's own preset untouched.
 */
export const THEME_LIQUID_ATTRIBUTE = 'data-theme-liquid'
export const THEME_LIQUID_VALUE = 'on'
export const THEME_LIQUID_PRESET: ThemePreset = 'liquid'

export function resolveLiquidMode(
  preset: ThemePreset,
  hasWallpaper: boolean
): boolean {
  return hasWallpaper || preset === THEME_LIQUID_PRESET
}

/**
 * How the wallpaper is cropped into the viewport.
 *
 * The address the user buys is only a URL, so "cropping" is a property of the
 * painting, not of the image: which point of the picture sits at the centre of
 * the screen, and how far in the zoom is. Kept out of the paid record on
 * purpose — reframing must stay free, and it is a per-browser preference like
 * the other appearance axes.
 */
export type WallpaperFraming = {
  /** 1 shows exactly what `cover` shows; above that crops in. */
  zoom: number
  /** Point of the image pinned to the centre, as a percentage of its size. */
  focusX: number
  focusY: number
  /**
   * `cover` fills the viewport and crops whatever does not fit; `contain` shows
   * the whole picture and leaves the rest of the screen to the theme. A wide
   * picture that cannot be seen in full at `cover` is the reason this exists.
   */
  fit: WallpaperFit
}

export type WallpaperFit = 'cover' | 'contain'

export const WALLPAPER_FIT_VALUES: ReadonlySet<WallpaperFit> = new Set([
  'cover',
  'contain',
])

export const WALLPAPER_ZOOM_MIN = 1
export const WALLPAPER_ZOOM_MAX = 4

export const DEFAULT_WALLPAPER_FRAMING: WallpaperFraming = {
  zoom: WALLPAPER_ZOOM_MIN,
  focusX: 50,
  focusY: 50,
  fit: 'cover',
}

/**
 * How much of the wallpaper the console's surfaces let through, 0-100. At 0 the
 * surfaces are opaque and the wallpaper is hidden; at 100 they are clear. It
 * exists because "readable" and "visible" pull against each other and only the
 * user can say where the balance belongs for the picture they chose.
 */
export const WALLPAPER_VISIBILITY_DEFAULT = 45
export const WALLPAPER_VISIBILITY_MIN = 0
export const WALLPAPER_VISIBILITY_MAX = 90

export const THEME_WALLPAPER_COOKIE_KEYS = {
  framing: 'theme_wallpaper_framing',
  accent: 'theme_wallpaper_accent',
  visibility: 'theme_wallpaper_visibility',
} as const

/** Set on <body> when the wallpaper's own colour is driving the accent. */
export const THEME_WALLPAPER_ACCENT_ATTRIBUTE = 'data-theme-wallpaper-accent'
export const THEME_WALLPAPER_ACCENT_VALUE = 'derived'
export const THEME_WALLPAPER_ACCENT_VARIABLES = {
  accent: '--wallpaper-accent',
  foreground: '--wallpaper-accent-foreground',
} as const

/** Where the crop reads from, and how large the image is painted. */
export const THEME_WALLPAPER_POSITION_VARIABLE = '--wallpaper-position'
export const THEME_WALLPAPER_SIZE_VARIABLE = '--wallpaper-size'
/** Numeric zoom, for media that is scaled rather than sized (the video layer). */
export const THEME_WALLPAPER_ZOOM_VARIABLE = '--wallpaper-zoom'
/**
 * Complete colours for the console's frosted surfaces, written by the provider
 * from the visibility setting. Whole colours rather than a percentage, because a
 * `color-mix()` percentage written as `var()` is resolved away by the CSS
 * minifier and the surface comes back opaque.
 */
export const THEME_WALLPAPER_SURFACE_BG_VARIABLE = '--wallpaper-surface-bg'
export const THEME_WALLPAPER_HEADER_BG_VARIABLE = '--wallpaper-header-bg'

/** Marks a wallpaper whose address is a video rather than a picture. */
export const THEME_WALLPAPER_MEDIA_ATTRIBUTE = 'data-theme-wallpaper-media'
export const THEME_WALLPAPER_MEDIA_VIDEO = 'video'
/** Slot of the <video> layer the provider mounts for a video wallpaper. */
export const THEME_WALLPAPER_VIDEO_SLOT = 'wallpaper-video'
/** How the picture is fitted, so the video layer can match the CSS layer. */
export const THEME_WALLPAPER_FIT_ATTRIBUTE = 'data-theme-wallpaper-fit'
/** Slot of the drag-to-frame overlay while the user is placing the picture. */
export const THEME_WALLPAPER_FRAMING_SLOT = 'wallpaper-framing-overlay'

/** Containers a wallpaper address may point at. */
export const WALLPAPER_VIDEO_EXTENSIONS = [
  '.mp4',
  '.webm',
  '.ogv',
  '.mov',
  '.m4v',
] as const

/**
 * Whether the address is a video rather than a picture.
 *
 * The field takes any address and nothing reports its content type before the
 * browser fetches it, so the extension is the signal. Query and fragment are
 * ignored, since a CDN address usually carries them.
 */
export function isVideoWallpaperUrl(raw: string): boolean {
  if (raw === '') return false
  let path: string
  try {
    path = new URL(raw).pathname
  } catch {
    return false
  }
  const lower = path.toLowerCase()
  return WALLPAPER_VIDEO_EXTENSIONS.some((extension) => lower.endsWith(extension))
}

/** The only value the accent cookie stores when the user turns it on. */
export const WALLPAPER_ACCENT_ENABLED = 'on'

function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, value))
}

export function normalizeWallpaperFraming(
  framing: WallpaperFraming
): WallpaperFraming {
  return {
    zoom: clamp(
      Number.isFinite(framing.zoom) ? framing.zoom : WALLPAPER_ZOOM_MIN,
      WALLPAPER_ZOOM_MIN,
      WALLPAPER_ZOOM_MAX
    ),
    focusX: clamp(
      Number.isFinite(framing.focusX) ? framing.focusX : 50,
      0,
      100
    ),
    focusY: clamp(
      Number.isFinite(framing.focusY) ? framing.focusY : 50,
      0,
      100
    ),
    fit: WALLPAPER_FIT_VALUES.has(framing.fit)
      ? framing.fit
      : DEFAULT_WALLPAPER_FRAMING.fit,
  }
}

export function isDefaultWallpaperFraming(framing: WallpaperFraming): boolean {
  const normalized = normalizeWallpaperFraming(framing)
  return (
    normalized.zoom === DEFAULT_WALLPAPER_FRAMING.zoom &&
    normalized.focusX === DEFAULT_WALLPAPER_FRAMING.focusX &&
    normalized.focusY === DEFAULT_WALLPAPER_FRAMING.focusY &&
    normalized.fit === DEFAULT_WALLPAPER_FRAMING.fit
  )
}

/**
 * `zoom,focusX,focusY[,fit]`.
 *
 * The fit is optional so a cookie written before it existed still reads: its
 * absence simply means `cover`, which is what those sessions were showing.
 */
export function parseWallpaperFraming(
  raw: string | null | undefined
): WallpaperFraming {
  if (!raw) return DEFAULT_WALLPAPER_FRAMING
  const parts = raw.split(',')
  if (parts.length !== 3 && parts.length !== 4) {
    return DEFAULT_WALLPAPER_FRAMING
  }
  const [zoom, focusX, focusY] = parts.map((part) => Number(part.trim()))
  if (
    !Number.isFinite(zoom) ||
    !Number.isFinite(focusX) ||
    !Number.isFinite(focusY)
  ) {
    return DEFAULT_WALLPAPER_FRAMING
  }
  return normalizeWallpaperFraming({
    zoom,
    focusX,
    focusY,
    fit: (parts[3]?.trim() as WallpaperFit) ?? DEFAULT_WALLPAPER_FRAMING.fit,
  })
}

export function serializeWallpaperFraming(framing: WallpaperFraming): string {
  const normalized = normalizeWallpaperFraming(framing)
  return [
    normalized.zoom.toFixed(2),
    Math.round(normalized.focusX),
    Math.round(normalized.focusY),
    normalized.fit,
  ].join(',')
}

export function clampWallpaperVisibility(value: number): number {
  return clamp(
    Number.isFinite(value) ? value : WALLPAPER_VISIBILITY_DEFAULT,
    WALLPAPER_VISIBILITY_MIN,
    WALLPAPER_VISIBILITY_MAX
  )
}

export function parseWallpaperVisibility(
  raw: string | null | undefined
): number {
  if (!raw) return WALLPAPER_VISIBILITY_DEFAULT
  const parsed = Number(raw.trim())
  return Number.isFinite(parsed)
    ? clampWallpaperVisibility(parsed)
    : WALLPAPER_VISIBILITY_DEFAULT
}

/** The percentage the surfaces keep, i.e. the inverse of what shows through. */
export function wallpaperSurfaceOpacity(visibility: number): number {
  return 100 - clampWallpaperVisibility(visibility)
}

/**
 * The translucent surface colour for a given visibility.
 *
 * The header is a little denser than the rest: it is sticky, so content keeps
 * sliding under it and its controls have to stay readable.
 */
export function wallpaperSurfaceColor(
  visibility: number,
  purpose: 'surface' | 'header' = 'surface'
): string {
  const strength = clampWallpaperVisibility(visibility)
  const opacity = clamp(
    100 - strength - (purpose === 'header' ? 8 : 0),
    0,
    100
  )
  return `color-mix(in oklab, var(--background) ${opacity.toFixed(0)}%, transparent)`
}

/**
 * The `background-size` the wallpaper layer should use.
 *
 * At 1x this is `cover` or `contain` verbatim. Above 1x the size has to be
 * spelled out, because `background-size` has no "cover times N" form: `cover`
 * scales up whichever axis is short and `contain` whichever is long, so that is
 * the axis to multiply and knowing it needs the image's aspect ratio. While the
 * ratio is unknown the answer stays the plain keyword, so the layer is never
 * painted smaller than `cover` would have painted it.
 */
export function wallpaperBackgroundSize(
  zoom: number,
  imageAspect: number | null,
  viewportAspect: number | null,
  fit: WallpaperFit = 'cover'
): string {
  if (!(zoom > WALLPAPER_ZOOM_MIN)) return fit
  if (!imageAspect || !(imageAspect > 0)) return fit
  if (!viewportAspect || !(viewportAspect > 0)) return fit
  const z = clamp(zoom, WALLPAPER_ZOOM_MIN, WALLPAPER_ZOOM_MAX)
  const imageIsWider = imageAspect > viewportAspect
  // `cover` fits the short axis (an image wider than the viewport is
  // height-constrained); `contain` fits the long one, the other way round.
  const fitHeight = fit === 'cover' ? imageIsWider : !imageIsWider
  return fitHeight ? `auto calc(100% * ${z})` : `calc(100% * ${z}) auto`
}

/**
 * Preset → default font mapping. Used by the provider to resolve the user's
 * `font: 'default'` preference against the active preset.
 *
 * Co-located with the preset registry so a preset's signature typography
 * is declared in one place. Presets not listed here fall back to the
 * `resolveThemeFont` default of `sans`. The shipped `default` preset
 * opts into serif so the editorial Lora voice is the out-of-the-box
 * experience; vivid color presets stay on the humanist sans so their
 * accents read clearly without competing with the body type.
 */
export const PRESET_DEFAULT_FONT: Partial<
  Record<ThemePreset, ResolvedThemeFont>
> = {
  default: 'sans',
  anthropic: 'serif',
}

/**
 * Resolve a user font preference + active preset into the concrete font that
 * should drive the DOM. Pure function so it's safe to call inside both the
 * effect that applies the attribute and the UI preview that hints at what
 * `default` will render as.
 */
export function resolveThemeFont(
  font: ThemeFont,
  preset: ThemePreset
): ResolvedThemeFont {
  if (font === 'default') {
    return PRESET_DEFAULT_FONT[preset] ?? 'sans'
  }
  return font
}
