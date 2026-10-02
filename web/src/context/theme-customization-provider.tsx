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
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'

import { sampleVideoAccent, sampleWallpaperAccent } from '@/features/wallpaper/lib/dominant-color'
import { getCookie, removeCookie, setCookie } from '@/lib/cookies'
import {
  CONTENT_LAYOUT_VALUES,
  type ContentLayout,
  DEFAULT_THEME_CUSTOMIZATION,
  DEFAULT_WALLPAPER_FRAMING,
  isDefaultWallpaperFraming,
  normalizeWallpaperFraming,
  parseWallpaperFraming,
  resolveLiquidMode,
  resolveThemeFont,
  serializeWallpaperFraming,
  THEME_COOKIE_KEYS,
  THEME_FONT_VALUES,
  THEME_LIQUID_ATTRIBUTE,
  THEME_LIQUID_VALUE,
  THEME_PRESET_VALUES,
  THEME_RADIUS_VALUES,
  THEME_SCALE_VALUES,
  THEME_WALLPAPER_ACCENT_ATTRIBUTE,
  THEME_WALLPAPER_ACCENT_VALUE,
  THEME_WALLPAPER_ACCENT_VARIABLES,
  THEME_WALLPAPER_ATTRIBUTE,
  THEME_WALLPAPER_COOKIE_KEYS,
  THEME_WALLPAPER_FIT_ATTRIBUTE,
  THEME_WALLPAPER_MEDIA_ATTRIBUTE,
  THEME_WALLPAPER_MEDIA_VIDEO,
  THEME_WALLPAPER_POSITION_VARIABLE,
  THEME_WALLPAPER_SIZE_VARIABLE,
  THEME_WALLPAPER_HEADER_BG_VARIABLE,
  THEME_WALLPAPER_SURFACE_BG_VARIABLE,
  THEME_WALLPAPER_VALUE,
  THEME_WALLPAPER_VIDEO_SLOT,
  THEME_WALLPAPER_ZOOM_VARIABLE,
  clampWallpaperVisibility,
  parseWallpaperVisibility,
  isVideoWallpaperUrl,
  wallpaperSurfaceColor,
  WALLPAPER_VISIBILITY_DEFAULT,
  THEME_WALLPAPER_VARIABLE,
  type ThemeCustomization,
  type ThemeFont,
  type ThemePreset,
  type ThemeRadius,
  type ThemeScale,
  WALLPAPER_ACCENT_ENABLED,
  wallpaperBackgroundSize,
  type WallpaperFraming,
} from '@/lib/theme-customization'
import { useSystemConfigStore } from '@/stores/system-config-store'

const COOKIE_MAX_AGE = 60 * 60 * 24 * 365 // 1 year

function readCookie<T extends string>(
  name: string,
  allowed: ReadonlySet<T>,
  fallback: T
): T {
  const value = getCookie(name)
  return value && allowed.has(value as T) ? (value as T) : fallback
}

function applyAttribute(name: string, value: string | null) {
  if (typeof document === 'undefined') return
  const body = document.body
  if (!body) return
  if (value === null) {
    body.removeAttribute(name)
  } else {
    body.setAttribute(name, value)
  }
}

type ThemeCustomizationContextType = {
  defaults: ThemeCustomization
  customization: ThemeCustomization
  /** The caller's paid wallpaper, empty when none is set. */
  wallpaperUrl: string
/** Whether the liquid (glass) surface layer is active. */
  liquid: boolean
  /** Crop of the wallpaper. Free and per-browser, unlike the address itself. */
  wallpaperFraming: WallpaperFraming
  /** Whether the wallpaper's own colour drives the accent. */
  wallpaperAccentEnabled: boolean
  /** How much of the wallpaper the console's surfaces let through, 0-100. */
  wallpaperVisibility: number
  /** Whether the whole page is currently a drag surface for framing. */
  wallpaperFramingMode: boolean
  setWallpaper: (wallpaperUrl: string) => void
  setWallpaperFraming: (framing: WallpaperFraming) => void
  resetWallpaperFraming: () => void
  setWallpaperVisibility: (visibility: number) => void
  setWallpaperAccentEnabled: (enabled: boolean) => void
  setWallpaperFramingMode: (active: boolean) => void
  setPreset: (preset: ThemePreset) => void
  setFont: (font: ThemeFont) => void
  setRadius: (radius: ThemeRadius) => void
  setScale: (scale: ThemeScale) => void
  setContentLayout: (contentLayout: ContentLayout) => void
  resetPreset: () => void
  resetFont: () => void
  resetRadius: () => void
  resetScale: () => void
  resetContentLayout: () => void
  resetCustomization: () => void
}

// Fallback used when a consumer renders outside the provider (e.g. an error
// route mounted before providers are ready, or stale HMR boundaries). Keeping
// it permissive prevents the whole tree from crashing — the UI just behaves
// like the defaults until the real provider re-mounts.
const FALLBACK_CONTEXT: ThemeCustomizationContextType = {
  defaults: DEFAULT_THEME_CUSTOMIZATION,
  customization: DEFAULT_THEME_CUSTOMIZATION,
  wallpaperUrl: '',
liquid: false,
  wallpaperFraming: DEFAULT_WALLPAPER_FRAMING,
  wallpaperAccentEnabled: false,
  wallpaperVisibility: WALLPAPER_VISIBILITY_DEFAULT,
  wallpaperFramingMode: false,
  setWallpaper: () => {},
  setWallpaperFraming: () => {},
  resetWallpaperFraming: () => {},
  setWallpaperVisibility: () => {},
  setWallpaperAccentEnabled: () => {},
  setWallpaperFramingMode: () => {},
  setPreset: () => {},
  setFont: () => {},
  setRadius: () => {},
  setScale: () => {},
  setContentLayout: () => {},
  resetPreset: () => {},
  resetFont: () => {},
  resetRadius: () => {},
  resetScale: () => {},
  resetContentLayout: () => {},
  resetCustomization: () => {},
}

const ThemeCustomizationContext =
  createContext<ThemeCustomizationContextType>(FALLBACK_CONTEXT)

export function ThemeCustomizationProvider(props: {
  children: React.ReactNode
}) {
  const appearance = useSystemConfigStore((state) => state.config.appearance)
  const defaults = useMemo<ThemeCustomization>(
    () => ({
      preset: THEME_PRESET_VALUES.has(appearance?.defaultThemePreset)
        ? appearance.defaultThemePreset
        : DEFAULT_THEME_CUSTOMIZATION.preset,
      font: THEME_FONT_VALUES.has(appearance?.defaultThemeFont)
        ? appearance.defaultThemeFont
        : DEFAULT_THEME_CUSTOMIZATION.font,
      radius: THEME_RADIUS_VALUES.has(appearance?.defaultThemeRadius)
        ? appearance.defaultThemeRadius
        : DEFAULT_THEME_CUSTOMIZATION.radius,
      scale: THEME_SCALE_VALUES.has(appearance?.defaultThemeScale)
        ? appearance.defaultThemeScale
        : DEFAULT_THEME_CUSTOMIZATION.scale,
      contentLayout: CONTENT_LAYOUT_VALUES.has(appearance?.defaultContentLayout)
        ? appearance.defaultContentLayout
        : DEFAULT_THEME_CUSTOMIZATION.contentLayout,
    }),
    [appearance]
  )
  const [preset, _setPreset] = useState<ThemePreset>(() =>
    readCookie<ThemePreset>(
      THEME_COOKIE_KEYS.preset,
      THEME_PRESET_VALUES,
      defaults.preset
    )
  )
  const [font, _setFont] = useState<ThemeFont>(() =>
    readCookie<ThemeFont>(
      THEME_COOKIE_KEYS.font,
      THEME_FONT_VALUES,
      defaults.font
    )
  )
  const [radius, _setRadius] = useState<ThemeRadius>(() =>
    readCookie<ThemeRadius>(
      THEME_COOKIE_KEYS.radius,
      THEME_RADIUS_VALUES,
      defaults.radius
    )
  )
  const [scale, _setScale] = useState<ThemeScale>(() =>
    readCookie<ThemeScale>(
      THEME_COOKIE_KEYS.scale,
      THEME_SCALE_VALUES,
      defaults.scale
    )
  )
  const [contentLayout, _setContentLayout] = useState<ContentLayout>(() =>
    readCookie<ContentLayout>(
      THEME_COOKIE_KEYS.contentLayout,
      CONTENT_LAYOUT_VALUES,
      defaults.contentLayout
    )
  )
  const [wallpaperUrl, setWallpaper] = useState('')
  const [wallpaperFraming, _setWallpaperFraming] = useState<WallpaperFraming>(
    () => parseWallpaperFraming(getCookie(THEME_WALLPAPER_COOKIE_KEYS.framing))
  )
  const [wallpaperAccentEnabled, _setWallpaperAccentEnabled] = useState(
    () =>
      getCookie(THEME_WALLPAPER_COOKIE_KEYS.accent) === WALLPAPER_ACCENT_ENABLED
  )
  const [wallpaperVisibility, _setWallpaperVisibility] = useState(() =>
    parseWallpaperVisibility(
      getCookie(THEME_WALLPAPER_COOKIE_KEYS.visibility)
    )
  )
  // While this is on, the whole viewport becomes a drag surface for placing the
  // picture. Off by default: dragging the page is how people scroll and select.
  const [wallpaperFramingMode, setWallpaperFramingMode] = useState(false)
  // Natural aspect ratio of the wallpaper. Only needed to spell out a zoomed
  // `background-size`; stays null until the image has been measured, and for an
  // image that never loads.
  const [wallpaperAspect, setWallpaperAspect] = useState<number | null>(null)

  // A video is painted by a <video> layer rather than as a background image, so
  // the whole wall paper path branches on this.
  const wallpaperIsVideo = isVideoWallpaperUrl(wallpaperUrl)

  useEffect(() => {
    if (!getCookie(THEME_COOKIE_KEYS.preset)) _setPreset(defaults.preset)
    if (!getCookie(THEME_COOKIE_KEYS.font)) _setFont(defaults.font)
    if (!getCookie(THEME_COOKIE_KEYS.radius)) _setRadius(defaults.radius)
    if (!getCookie(THEME_COOKIE_KEYS.scale)) _setScale(defaults.scale)
    if (!getCookie(THEME_COOKIE_KEYS.contentLayout)) {
      _setContentLayout(defaults.contentLayout)
    }
  }, [defaults])

  // Mirror state to the <body> via data-* attributes so theme-presets.css can
  // override CSS variables at the right cascade layer.
  useEffect(() => {
    applyAttribute(
      'data-theme-preset',
      preset === DEFAULT_THEME_CUSTOMIZATION.preset ? null : preset
    )
  }, [preset])

  // Font is the one axis where we resolve before writing the attribute:
  // the persisted preference may be `default`, but CSS works in terms of
  // the concrete `sans`/`serif` choice that should drive the cascade.
  // Resolving here (instead of in CSS via `:not()` selectors) keeps the
  // stylesheet to one simple `[data-theme-font='serif']` selector and lets
  // future presets opt into typography via `PRESET_DEFAULT_FONT` alone.
  useEffect(() => {
    applyAttribute('data-theme-font', resolveThemeFont(font, preset))
  }, [font, preset])

  useEffect(() => {
    applyAttribute(
      'data-theme-radius',
      radius === DEFAULT_THEME_CUSTOMIZATION.radius ? null : radius
    )
  }, [radius])

  useEffect(() => {
    applyAttribute(
      'data-theme-scale',
      scale === DEFAULT_THEME_CUSTOMIZATION.scale ? null : scale
    )
  }, [scale])

  useEffect(() => {
    applyAttribute('data-theme-content-layout', contentLayout)
  }, [contentLayout])

  // The wallpaper is painted through a body-scoped CSS variable rather than an
  // inline style on the provider's children, so it also covers the header and
  // the area around the console's inset panel. The address was validated
  // against quotes, parentheses and backslashes before it was stored, so it
  // cannot break out of the `url("...")` it is quoted into.
  useEffect(() => {
    if (typeof document === 'undefined') return
    const body = document.body
    if (!body) return
    if (wallpaperUrl === '') {
      body.removeAttribute(THEME_WALLPAPER_ATTRIBUTE)
      body.removeAttribute(THEME_WALLPAPER_MEDIA_ATTRIBUTE)
      body.removeAttribute(THEME_WALLPAPER_FIT_ATTRIBUTE)
      body.style.removeProperty(THEME_WALLPAPER_VARIABLE)
      body.style.removeProperty(THEME_WALLPAPER_POSITION_VARIABLE)
      body.style.removeProperty(THEME_WALLPAPER_SIZE_VARIABLE)
      body.style.removeProperty(THEME_WALLPAPER_ZOOM_VARIABLE)
      return
    }
    const viewportAspect =
      typeof window === 'undefined' || window.innerHeight === 0
        ? null
        : window.innerWidth / window.innerHeight
    const { zoom, focusX, focusY, fit } = wallpaperFraming
    body.setAttribute(THEME_WALLPAPER_ATTRIBUTE, THEME_WALLPAPER_VALUE)
    body.setAttribute(THEME_WALLPAPER_FIT_ATTRIBUTE, fit)
    body.style.setProperty(
      THEME_WALLPAPER_POSITION_VARIABLE,
      `${focusX}% ${focusY}%`
    )
    body.style.setProperty(THEME_WALLPAPER_ZOOM_VARIABLE, String(zoom))

    if (wallpaperIsVideo) {
      // A video cannot be a CSS background: the layer below paints it, and the
      // crop reaches it through object-position and the zoom variable instead.
      body.removeAttribute(THEME_WALLPAPER_VARIABLE)
      body.style.removeProperty(THEME_WALLPAPER_VARIABLE)
      body.style.removeProperty(THEME_WALLPAPER_SIZE_VARIABLE)
      body.setAttribute(
        THEME_WALLPAPER_MEDIA_ATTRIBUTE,
        THEME_WALLPAPER_MEDIA_VIDEO
      )
      return
    }

    body.removeAttribute(THEME_WALLPAPER_MEDIA_ATTRIBUTE)
    body.style.setProperty(THEME_WALLPAPER_VARIABLE, `url("${wallpaperUrl}")`)
    body.style.setProperty(
      THEME_WALLPAPER_SIZE_VARIABLE,
      wallpaperBackgroundSize(zoom, wallpaperAspect, viewportAspect, fit)
    )
  }, [wallpaperUrl, wallpaperIsVideo, wallpaperFraming, wallpaperAspect])

  // How much of the picture the console's surfaces let through. The whole
  // colour is written here rather than a percentage, because a `color-mix()`
  // percentage expressed as a `var()` gets resolved away by the CSS minifier and
  // the surface comes back opaque.
  useEffect(() => {
    if (typeof document === 'undefined') return
    const body = document.body
    if (!body) return
    if (wallpaperUrl === '') {
      body.style.removeProperty(THEME_WALLPAPER_SURFACE_BG_VARIABLE)
      body.style.removeProperty(THEME_WALLPAPER_HEADER_BG_VARIABLE)
      return
    }
    body.style.setProperty(
      THEME_WALLPAPER_SURFACE_BG_VARIABLE,
      wallpaperSurfaceColor(wallpaperVisibility)
    )
    body.style.setProperty(
      THEME_WALLPAPER_HEADER_BG_VARIABLE,
      wallpaperSurfaceColor(wallpaperVisibility, 'header')
    )
  }, [wallpaperUrl, wallpaperVisibility])

  // The <video> layer for a video wallpaper, mounted as a child of <body> so the
  // console's own surfaces paint over it.
  useEffect(() => {
    if (typeof document === 'undefined') return
    const body = document.body
    if (!body) return
    if (!wallpaperIsVideo || wallpaperUrl === '') return

    const video = document.createElement('video')
    video.setAttribute('data-slot', THEME_WALLPAPER_VIDEO_SLOT)
    video.setAttribute('aria-hidden', 'true')
    video.setAttribute('tabindex', '-1')
    video.src = wallpaperUrl
    // Both the property and the attribute: an autoplay policy reads the
    // attribute, and a muted video is the one it lets start on its own.
    video.muted = true
    video.setAttribute('muted', '')
    video.loop = true
    video.autoplay = true
    video.playsInline = true
    video.preload = 'auto'

    // Autoplay is the browser's decision, not ours; the first user gesture is
    // the standard second attempt when it declines.
    const tryPlay = () => {
      void video.play().catch(() => {})
    }
    tryPlay()
    window.addEventListener('pointerdown', tryPlay, { once: true })
    body.prepend(video)

    return () => {
      window.removeEventListener('pointerdown', tryPlay)
      video.pause()
      video.removeAttribute('src')
      video.remove()
    }
  }, [wallpaperUrl, wallpaperIsVideo])

  // Measuring the wallpaper needs no CORS permission -- only reading its pixels
  // does -- so the crop works even for a host that never sends CORS headers.
  // A video is scaled rather than sized, so it needs no aspect ratio.
  useEffect(() => {
    if (wallpaperUrl === '' || wallpaperIsVideo) {
      setWallpaperAspect(null)
      return
    }
    let cancelled = false
    const image = new Image()
    image.referrerPolicy = 'no-referrer'
    image.addEventListener(
      'load',
      () => {
        if (cancelled) return
        setWallpaperAspect(
          image.naturalHeight > 0
            ? image.naturalWidth / image.naturalHeight
            : null
        )
      },
      { once: true }
    )
    image.addEventListener(
      'error',
      () => {
        if (!cancelled) setWallpaperAspect(null)
      },
      { once: true }
    )
    image.src = wallpaperUrl
    return () => {
      cancelled = true
    }
  }, [wallpaperUrl, wallpaperIsVideo])

  // Deriving the accent from the wallpaper is opt-in, and silently a no-op when
  // the image cannot be read: the preset's own accent simply stays in place.
  useEffect(() => {
    if (typeof document === 'undefined') return
    const body = document.body
    if (!body) return
    const clearAccent = () => {
      body.removeAttribute(THEME_WALLPAPER_ACCENT_ATTRIBUTE)
      body.style.removeProperty(THEME_WALLPAPER_ACCENT_VARIABLES.accent)
      body.style.removeProperty(THEME_WALLPAPER_ACCENT_VARIABLES.foreground)
    }
    if (wallpaperUrl === '' || !wallpaperAccentEnabled) {
      clearAccent()
      return
    }
    let cancelled = false
    const sample = wallpaperIsVideo ? sampleVideoAccent : sampleWallpaperAccent
    void sample(wallpaperUrl).then((accent) => {
      if (cancelled) return
      if (!accent) {
        clearAccent()
        return
      }
      body.style.setProperty(
        THEME_WALLPAPER_ACCENT_VARIABLES.accent,
        accent.accent
      )
      body.style.setProperty(
        THEME_WALLPAPER_ACCENT_VARIABLES.foreground,
        accent.foreground
      )
      body.setAttribute(
        THEME_WALLPAPER_ACCENT_ATTRIBUTE,
        THEME_WALLPAPER_ACCENT_VALUE
      )
    })
    return () => {
      cancelled = true
    }
  }, [wallpaperUrl, wallpaperAccentEnabled, wallpaperIsVideo])

  // Liquid is derived rather than stored: a wallpaper turns it on, and picking
  // the Liquid Glass preset turns it on without one. Because nothing is written
  // to a cookie, removing the wallpaper drops straight back to the preset the
  // user had chosen.
  const liquid = resolveLiquidMode(preset, wallpaperUrl !== '')

  useEffect(() => {
    applyAttribute(THEME_LIQUID_ATTRIBUTE, liquid ? THEME_LIQUID_VALUE : null)
  }, [liquid])

  const setPreset = useCallback((value: ThemePreset) => {
    _setPreset(value)
    setCookie(THEME_COOKIE_KEYS.preset, value, COOKIE_MAX_AGE)
  }, [])

  const setFont = useCallback((value: ThemeFont) => {
    _setFont(value)
    setCookie(THEME_COOKIE_KEYS.font, value, COOKIE_MAX_AGE)
  }, [])

  const setRadius = useCallback((value: ThemeRadius) => {
    _setRadius(value)
    setCookie(THEME_COOKIE_KEYS.radius, value, COOKIE_MAX_AGE)
  }, [])

  const setScale = useCallback((value: ThemeScale) => {
    _setScale(value)
    setCookie(THEME_COOKIE_KEYS.scale, value, COOKIE_MAX_AGE)
  }, [])

  const setContentLayout = useCallback((value: ContentLayout) => {
    _setContentLayout(value)
    setCookie(THEME_COOKIE_KEYS.contentLayout, value, COOKIE_MAX_AGE)
  }, [])

  const setWallpaperFraming = useCallback((next: WallpaperFraming) => {
    const normalized = normalizeWallpaperFraming(next)
    _setWallpaperFraming(normalized)
    if (isDefaultWallpaperFraming(normalized)) {
      removeCookie(THEME_WALLPAPER_COOKIE_KEYS.framing)
    } else {
      setCookie(
        THEME_WALLPAPER_COOKIE_KEYS.framing,
        serializeWallpaperFraming(normalized),
        COOKIE_MAX_AGE
      )
    }
  }, [])

  const resetWallpaperFraming = useCallback(() => {
    setWallpaperFraming(DEFAULT_WALLPAPER_FRAMING)
  }, [setWallpaperFraming])

  const setWallpaperAccentEnabled = useCallback((enabled: boolean) => {
    _setWallpaperAccentEnabled(enabled)
    if (enabled) {
      setCookie(
        THEME_WALLPAPER_COOKIE_KEYS.accent,
        WALLPAPER_ACCENT_ENABLED,
        COOKIE_MAX_AGE
      )
    } else {
      removeCookie(THEME_WALLPAPER_COOKIE_KEYS.accent)
    }
  }, [])

  const setWallpaperVisibility = useCallback((value: number) => {
    const normalized = clampWallpaperVisibility(value)
    _setWallpaperVisibility(normalized)
    if (normalized === WALLPAPER_VISIBILITY_DEFAULT) {
      removeCookie(THEME_WALLPAPER_COOKIE_KEYS.visibility)
    } else {
      setCookie(
        THEME_WALLPAPER_COOKIE_KEYS.visibility,
        String(normalized),
        COOKIE_MAX_AGE
      )
    }
  }, [])

  // The wallpaper is deliberately left alone: it was bought, so "reset" must
  // not silently unpaint something the user paid for. Removing it is its own
  // explicit action in the wallpaper settings.
  const resetPreset = useCallback(() => {
    removeCookie(THEME_COOKIE_KEYS.preset)
    _setPreset(defaults.preset)
  }, [defaults.preset])
  const resetFont = useCallback(() => {
    removeCookie(THEME_COOKIE_KEYS.font)
    _setFont(defaults.font)
  }, [defaults.font])
  const resetRadius = useCallback(() => {
    removeCookie(THEME_COOKIE_KEYS.radius)
    _setRadius(defaults.radius)
  }, [defaults.radius])
  const resetScale = useCallback(() => {
    removeCookie(THEME_COOKIE_KEYS.scale)
    _setScale(defaults.scale)
  }, [defaults.scale])
  const resetContentLayout = useCallback(() => {
    removeCookie(THEME_COOKIE_KEYS.contentLayout)
    _setContentLayout(defaults.contentLayout)
  }, [defaults.contentLayout])

  const resetCustomization = useCallback(() => {
    resetPreset()
    resetFont()
    resetRadius()
    resetScale()
    resetContentLayout()
  }, [resetPreset, resetFont, resetRadius, resetScale, resetContentLayout])

  const value = useMemo<ThemeCustomizationContextType>(
    () => ({
      defaults,
      customization: { preset, font, radius, scale, contentLayout },
      wallpaperUrl,
      liquid,
      wallpaperFraming,
      wallpaperAccentEnabled,
      wallpaperVisibility,
      wallpaperFramingMode,
      setWallpaper,
      setWallpaperFraming,
      resetWallpaperFraming,
      setWallpaperVisibility,
      setWallpaperAccentEnabled,
      setWallpaperFramingMode,
      setPreset,
      setFont,
      setRadius,
      setScale,
      setContentLayout,
      resetPreset,
      resetFont,
      resetRadius,
      resetScale,
      resetContentLayout,
      resetCustomization,
    }),
    [
      preset,
      font,
      radius,
      scale,
      contentLayout,
      wallpaperUrl,
      liquid,
      wallpaperFraming,
      wallpaperAccentEnabled,
      wallpaperVisibility,
      wallpaperFramingMode,
      setWallpaper,
      setWallpaperFraming,
      resetWallpaperFraming,
      setWallpaperVisibility,
      setWallpaperAccentEnabled,
      setPreset,
      setFont,
      setRadius,
      setScale,
      setContentLayout,
      resetPreset,
      resetFont,
      resetRadius,
      resetScale,
      resetContentLayout,
      resetCustomization,
      defaults,
    ]
  )

  return (
    <ThemeCustomizationContext.Provider value={value}>
      {props.children}
    </ThemeCustomizationContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useThemeCustomization() {
  return useContext(ThemeCustomizationContext)
}
