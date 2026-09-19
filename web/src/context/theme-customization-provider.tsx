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

import { sampleWallpaperAccent } from '@/features/wallpaper/lib/dominant-color'
import { getCookie, removeCookie, setCookie } from '@/lib/cookies'
import {
  CONTENT_LAYOUT_VALUES,
  type ContentLayout,
  DEFAULT_THEME_CUSTOMIZATION,
  DEFAULT_WALLPAPER_FRAMING,
  isDefaultWallpaperFraming,
  normalizeWallpaperFraming,
  parseWallpaperFraming,
  resolveThemeFont,
  serializeWallpaperFraming,
  THEME_COOKIE_KEYS,
  THEME_FONT_VALUES,
  THEME_PRESET_VALUES,
  THEME_RADIUS_VALUES,
  THEME_SCALE_VALUES,
  THEME_WALLPAPER_ACCENT_ATTRIBUTE,
  THEME_WALLPAPER_ACCENT_VALUE,
  THEME_WALLPAPER_ACCENT_VARIABLES,
  THEME_WALLPAPER_ATTRIBUTE,
  THEME_WALLPAPER_COOKIE_KEYS,
  THEME_WALLPAPER_POSITION_VARIABLE,
  THEME_WALLPAPER_SIZE_VARIABLE,
  THEME_WALLPAPER_VALUE,
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
  /** Crop of the wallpaper. Free and per-browser, unlike the address itself. */
  wallpaperFraming: WallpaperFraming
  /** Whether the wallpaper's own colour drives the accent. */
  wallpaperAccentEnabled: boolean
  setWallpaper: (wallpaperUrl: string) => void
  setWallpaperFraming: (framing: WallpaperFraming) => void
  resetWallpaperFraming: () => void
  setWallpaperAccentEnabled: (enabled: boolean) => void
  setPreset: (preset: ThemePreset) => void
  setFont: (font: ThemeFont) => void
  setRadius: (radius: ThemeRadius) => void
  setScale: (scale: ThemeScale) => void
  setContentLayout: (contentLayout: ContentLayout) => void
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
  wallpaperFraming: DEFAULT_WALLPAPER_FRAMING,
  wallpaperAccentEnabled: false,
  setWallpaper: () => {},
  setWallpaperFraming: () => {},
  resetWallpaperFraming: () => {},
  setWallpaperAccentEnabled: () => {},
  setPreset: () => {},
  setFont: () => {},
  setRadius: () => {},
  setScale: () => {},
  setContentLayout: () => {},
  resetCustomization: () => {},
}

const ThemeCustomizationContext =
  createContext<ThemeCustomizationContextType>(FALLBACK_CONTEXT)

export function ThemeCustomizationProvider(props: {
  children: React.ReactNode
}) {
  const [preset, _setPreset] = useState<ThemePreset>(() =>
    readCookie<ThemePreset>(
      THEME_COOKIE_KEYS.preset,
      THEME_PRESET_VALUES,
      DEFAULT_THEME_CUSTOMIZATION.preset
    )
  )
  const [font, _setFont] = useState<ThemeFont>(() =>
    readCookie<ThemeFont>(
      THEME_COOKIE_KEYS.font,
      THEME_FONT_VALUES,
      DEFAULT_THEME_CUSTOMIZATION.font
    )
  )
  const [radius, _setRadius] = useState<ThemeRadius>(() =>
    readCookie<ThemeRadius>(
      THEME_COOKIE_KEYS.radius,
      THEME_RADIUS_VALUES,
      DEFAULT_THEME_CUSTOMIZATION.radius
    )
  )
  const [scale, _setScale] = useState<ThemeScale>(() =>
    readCookie<ThemeScale>(
      THEME_COOKIE_KEYS.scale,
      THEME_SCALE_VALUES,
      DEFAULT_THEME_CUSTOMIZATION.scale
    )
  )
  const [contentLayout, _setContentLayout] = useState<ContentLayout>(() =>
    readCookie<ContentLayout>(
      THEME_COOKIE_KEYS.contentLayout,
      CONTENT_LAYOUT_VALUES,
      DEFAULT_THEME_CUSTOMIZATION.contentLayout
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
  // Natural aspect ratio of the wallpaper. Only needed to spell out a zoomed
  // `background-size`; stays null until the image has been measured, and for an
  // image that never loads.
  const [wallpaperAspect, setWallpaperAspect] = useState<number | null>(null)

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
      body.style.removeProperty(THEME_WALLPAPER_VARIABLE)
      body.style.removeProperty(THEME_WALLPAPER_POSITION_VARIABLE)
      body.style.removeProperty(THEME_WALLPAPER_SIZE_VARIABLE)
      return
    }
    const viewportAspect =
      typeof window === 'undefined' || window.innerHeight === 0
        ? null
        : window.innerWidth / window.innerHeight
    const { zoom, focusX, focusY } = wallpaperFraming
    body.style.setProperty(THEME_WALLPAPER_VARIABLE, `url("${wallpaperUrl}")`)
    body.style.setProperty(
      THEME_WALLPAPER_POSITION_VARIABLE,
      `${focusX}% ${focusY}%`
    )
    body.style.setProperty(
      THEME_WALLPAPER_SIZE_VARIABLE,
      wallpaperBackgroundSize(zoom, wallpaperAspect, viewportAspect)
    )
    body.setAttribute(THEME_WALLPAPER_ATTRIBUTE, THEME_WALLPAPER_VALUE)
  }, [wallpaperUrl, wallpaperFraming, wallpaperAspect])

  // Measuring the wallpaper needs no CORS permission -- only reading its pixels
  // does -- so the crop works even for a host that never sends CORS headers.
  useEffect(() => {
    if (wallpaperUrl === '') {
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
  }, [wallpaperUrl])

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
    void sampleWallpaperAccent(wallpaperUrl).then((accent) => {
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
  }, [wallpaperUrl, wallpaperAccentEnabled])

  const setPreset = useCallback((value: ThemePreset) => {
    _setPreset(value)
    if (value === DEFAULT_THEME_CUSTOMIZATION.preset) {
      removeCookie(THEME_COOKIE_KEYS.preset)
    } else {
      setCookie(THEME_COOKIE_KEYS.preset, value, COOKIE_MAX_AGE)
    }
  }, [])

  const setFont = useCallback((value: ThemeFont) => {
    _setFont(value)
    if (value === DEFAULT_THEME_CUSTOMIZATION.font) {
      removeCookie(THEME_COOKIE_KEYS.font)
    } else {
      setCookie(THEME_COOKIE_KEYS.font, value, COOKIE_MAX_AGE)
    }
  }, [])

  const setRadius = useCallback((value: ThemeRadius) => {
    _setRadius(value)
    if (value === DEFAULT_THEME_CUSTOMIZATION.radius) {
      removeCookie(THEME_COOKIE_KEYS.radius)
    } else {
      setCookie(THEME_COOKIE_KEYS.radius, value, COOKIE_MAX_AGE)
    }
  }, [])

  const setScale = useCallback((value: ThemeScale) => {
    _setScale(value)
    if (value === DEFAULT_THEME_CUSTOMIZATION.scale) {
      removeCookie(THEME_COOKIE_KEYS.scale)
    } else {
      setCookie(THEME_COOKIE_KEYS.scale, value, COOKIE_MAX_AGE)
    }
  }, [])

  const setContentLayout = useCallback((value: ContentLayout) => {
    _setContentLayout(value)
    if (value === DEFAULT_THEME_CUSTOMIZATION.contentLayout) {
      removeCookie(THEME_COOKIE_KEYS.contentLayout)
    } else {
      setCookie(THEME_COOKIE_KEYS.contentLayout, value, COOKIE_MAX_AGE)
    }
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

  // The wallpaper is deliberately left alone: it was bought, so "reset" must
  // not silently unpaint something the user paid for. Removing it is its own
  // explicit action in the wallpaper settings.
  const resetCustomization = useCallback(() => {
    setPreset(DEFAULT_THEME_CUSTOMIZATION.preset)
    setFont(DEFAULT_THEME_CUSTOMIZATION.font)
    setRadius(DEFAULT_THEME_CUSTOMIZATION.radius)
    setScale(DEFAULT_THEME_CUSTOMIZATION.scale)
    setContentLayout(DEFAULT_THEME_CUSTOMIZATION.contentLayout)
  }, [setPreset, setFont, setRadius, setScale, setContentLayout])

  const value = useMemo<ThemeCustomizationContextType>(
    () => ({
      defaults: DEFAULT_THEME_CUSTOMIZATION,
      customization: { preset, font, radius, scale, contentLayout },
      wallpaperUrl,
      wallpaperFraming,
      wallpaperAccentEnabled,
      setWallpaper,
      setWallpaperFraming,
      resetWallpaperFraming,
      setWallpaperAccentEnabled,
      setPreset,
      setFont,
      setRadius,
      setScale,
      setContentLayout,
      resetCustomization,
    }),
    [
      preset,
      font,
      radius,
      scale,
      contentLayout,
      wallpaperUrl,
      wallpaperFraming,
      wallpaperAccentEnabled,
      setWallpaper,
      setWallpaperFraming,
      resetWallpaperFraming,
      setWallpaperAccentEnabled,
      setPreset,
      setFont,
      setRadius,
      setScale,
      setContentLayout,
      resetCustomization,
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
