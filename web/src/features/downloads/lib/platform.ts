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
import type {
  BuildArch,
  ClientArch,
  ClientPlatform,
  ClientTarget,
  DetectionSignals,
} from '../types'

const IOS_USER_AGENT = /iPhone|iPad|iPod/i
const ANDROID_USER_AGENT = /Android/i

/**
 * Read the platform from a User-Agent Client Hint.
 *
 * These hints are already normalised by the browser, so they beat any string
 * matching against the User-Agent. Phones, tablets and ChromeOS resolve to
 * `unknown` on purpose: no desktop build we publish targets them, and falling
 * through to the User-Agent would misfile them (an iPad's UA claims
 * "like Mac OS X").
 */
function platformFromClientHints(raw?: string | null): ClientPlatform | null {
  switch (raw) {
    case 'Windows':
      return 'windows'
    case 'macOS':
      return 'macos'
    case 'Linux':
      return 'linux'
    case 'Android':
    case 'iOS':
    case 'Chrome OS':
      return 'unknown'
    default:
      return null
  }
}

/**
 * Read the platform from the User-Agent string.
 *
 * iOS is checked before macOS because Safari's iPhone/iPad User-Agent embeds
 * "like Mac OS X"; without that ordering every iPhone would be offered the
 * desktop build.
 */
function platformFromUserAgent(raw?: string | null): ClientPlatform | null {
  if (!raw) return null

  if (IOS_USER_AGENT.test(raw)) return 'unknown'
  if (ANDROID_USER_AGENT.test(raw)) return 'unknown'
  if (/Windows/i.test(raw)) return 'windows'
  if (/Macintosh|Mac OS X/i.test(raw)) return 'macos'
  if (/Linux|X11/i.test(raw)) return 'linux'

  return null
}

/** Read the platform from the legacy, deprecated `navigator.platform`. */
function platformFromNavigatorPlatform(
  raw?: string | null
): ClientPlatform | null {
  if (!raw) return null

  if (/^Mac/i.test(raw)) return 'macos'
  if (/^Win/i.test(raw)) return 'windows'
  if (/^Linux/i.test(raw)) return 'linux'

  return null
}

function archFromClientHint(raw?: string | null): ClientArch | null {
  const value = raw?.toLowerCase()
  if (value === 'arm') return 'arm64'
  if (value === 'x86') return 'x64'

  return null
}

/**
 * Read the CPU architecture from the User-Agent string.
 *
 * `WOW64` and `Win64; x64` mark a 64-bit Windows build. That is ambiguous on
 * Windows ARM64, where Chromium reports `Win64; x64` because of x64
 * emulation, which is why the Client Hint above is preferred when the browser
 * exposes it.
 */
function archFromUserAgent(raw?: string | null): ClientArch | null {
  if (!raw) return null

  if (/aarch64|arm64/i.test(raw)) return 'arm64'
  if (/WOW64|Win64; x64|x86_64|amd64/i.test(raw)) return 'x64'

  return null
}

/**
 * Read the CPU architecture from `navigator.platform`.
 *
 * Only unambiguous tokens are accepted. `MacIntel` is deliberately ignored:
 * Safari reports it on Apple Silicon too, so treating it as x64 would label
 * every Apple Silicon Mac an Intel machine.
 */
function archFromNavigatorPlatform(raw?: string | null): ClientArch | null {
  if (!raw) return null

  // `armv7l`/`armv8l` on Linux and `MacARM` all carry a bare `arm` token, and
  // no other value this property reports contains that substring.
  if (/aarch64|arm64|arm/i.test(raw)) return 'arm64'
  if (/x86_64|amd64|win64/i.test(raw)) return 'x64'

  return null
}

export function detectClientPlatform(signals: DetectionSignals): ClientPlatform {
  return (
    platformFromClientHints(signals.clientHintPlatform) ??
    platformFromUserAgent(signals.userAgent) ??
    platformFromNavigatorPlatform(signals.platform) ??
    'unknown'
  )
}

export function detectClientArch(signals: DetectionSignals): ClientArch {
  return (
    archFromClientHint(signals.clientHintArchitecture) ??
    archFromUserAgent(signals.userAgent) ??
    archFromNavigatorPlatform(signals.platform) ??
    'unknown'
  )
}

export function detectClientTarget(signals: DetectionSignals): ClientTarget {
  return {
    platform: detectClientPlatform(signals),
    arch: detectClientArch(signals),
  }
}

/**
 * Whether a published build is the best match for the visitor's system.
 *
 * An unknown platform never matches, so the page shows no recommendation
 * rather than a wrong one. On Windows and Linux an unknown CPU is treated as
 * x64: it is the overwhelmingly common case, and every artifact stays visible
 * so an ARM visitor can still pick the correct one by hand.
 */
export function isRecommendedBuild(
  build: { platform: ClientPlatform; arch: BuildArch },
  target: ClientTarget
): boolean {
  if (target.platform === 'unknown') return false
  if (build.platform !== target.platform) return false
  if (build.arch === 'universal') return true

  const arch = target.arch === 'unknown' ? 'x64' : target.arch
  return build.arch === arch
}

/**
 * Chromium's User-Agent Client Hints API.
 *
 * It is not part of the DOM lib TypeScript ships with, so it is described
 * locally instead of widening `Navigator` globally.
 */
export type UserAgentDataLike = {
  platform?: string
  getHighEntropyValues?: (
    hints: readonly string[]
  ) => Promise<{ architecture?: string }>
}

export function readUserAgentData(): UserAgentDataLike | null {
  if (typeof navigator === 'undefined') return null

  return (
    (navigator as Navigator & { userAgentData?: UserAgentDataLike })
      .userAgentData ?? null
  )
}

export function readDetectionSignals(): DetectionSignals {
  if (typeof navigator === 'undefined') return {}

  return {
    userAgent: navigator.userAgent,
    platform: navigator.platform,
    clientHintPlatform: readUserAgentData()?.platform ?? null,
  }
}

/**
 * Ask for the high-entropy architecture hint, when the browser exposes it.
 *
 * Chromium answers this on Windows ARM64, where the User-Agent misleadingly
 * reports `Win64; x64` because of x64 emulation. Other engines resolve `null`
 * and keep the synchronous guess.
 */
export async function readHighEntropyArch(
  userAgentData: UserAgentDataLike | null
): Promise<ClientArch | null> {
  if (!userAgentData?.getHighEntropyValues) return null

  try {
    const values = await userAgentData.getHighEntropyValues(['architecture'])
    return archFromClientHint(values.architecture)
  } catch {
    return null
  }
}
