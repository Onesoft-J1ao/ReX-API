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
 * CC Switch release resolution.
 *
 * The download link must follow upstream releases instead of naming a version
 * in the source, so the current release is resolved at runtime:
 *
 *   1. the official Tauri manifest `https://dl.ccswitch.io/latest.json`, used
 *      only when it is reachable (see {@link resolveManifestUrl});
 *   2. the GitHub Releases API, which is the source that actually works from
 *      a browser because it sends `Access-Control-Allow-Origin: *`;
 *   3. {@link CC_SWITCH_FALLBACK_RELEASE} — a pinned release that keeps the
 *      page usable when both lookups fail (offline, rate limited, blocked).
 *
 * A resolved version is turned into real download URLs for the official CDN,
 * with the matching GitHub release asset kept as a mirror for the same file.
 */

export const CC_SWITCH_CDN_BASE = 'https://dl.ccswitch.io'
export const CC_SWITCH_MANIFEST_URL = `${CC_SWITCH_CDN_BASE}/latest.json`
export const CC_SWITCH_REPOSITORY = 'farion1231/cc-switch'
export const CC_SWITCH_RELEASES_API = `https://api.github.com/repos/${CC_SWITCH_REPOSITORY}/releases/latest`
export const CC_SWITCH_RELEASES_URL = `https://github.com/${CC_SWITCH_REPOSITORY}/releases`

/** Version used when no release source can be reached. Kept in sync by hand. */
export const CC_SWITCH_FALLBACK_VERSION = '3.20.3'

const MANIFEST_TIMEOUT_MS = 5000
const RELEASES_TIMEOUT_MS = 8000

export type CcSwitchSource = 'manifest' | 'github' | 'fallback'

export type CcSwitchAsset = {
  /** File name published by upstream, shared by the CDN and the mirror. */
  fileName: string
  /** Official CDN URL for the resolved version. */
  url: string
  /** GitHub release asset for the same file, used when the CDN fails. */
  mirrorUrl: string
}

export type CcSwitchRelease = {
  version: string
  source: CcSwitchSource
  assets: CcSwitchAsset[]
}

function stripVersionPrefix(raw: string): string {
  return raw.trim().replace(/^v/i, '')
}

function fileNameFromUrl(url: string): string | null {
  const path = url.split(/[?#]/)[0]
  return path.split('/').at(-1) || null
}

export function buildCdnUrl(version: string, fileName: string): string {
  return `${CC_SWITCH_CDN_BASE}/v${version}/${fileName}`
}

export function buildMirrorUrl(version: string, fileName: string): string {
  return `https://github.com/${CC_SWITCH_REPOSITORY}/releases/download/v${version}/${fileName}`
}

function buildAssets(
  version: string,
  files: readonly string[]
): CcSwitchAsset[] {
  return files.map((fileName) => ({
    fileName,
    url: buildCdnUrl(version, fileName),
    mirrorUrl: buildMirrorUrl(version, fileName),
  }))
}

const CC_SWITCH_FALLBACK_FILES = [
  'CC-Switch-v3.20.3-Windows.msi',
  'CC-Switch-v3.20.3-Windows-arm64.msi',
  'CC-Switch-v3.20.3-macOS.dmg',
  'CC-Switch-v3.20.3-Linux-x86_64.AppImage',
] as const

/**
 * Last-resort release, used when neither the manifest nor the GitHub API can
 * be reached. Its URLs were live when the pinned version was published, and
 * the UI tells the visitor that the live lookup failed.
 */
export const CC_SWITCH_FALLBACK_RELEASE: CcSwitchRelease = {
  version: CC_SWITCH_FALLBACK_VERSION,
  source: 'fallback',
  assets: buildAssets(CC_SWITCH_FALLBACK_VERSION, CC_SWITCH_FALLBACK_FILES),
}

/**
 * Parse the official `latest.json` Tauri manifest.
 *
 * Every desktop platform variant points at the same artifact per architecture
 * on macOS, so assets are de-duplicated by file name.
 */
export function parseCcSwitchManifest(raw: unknown): CcSwitchRelease | null {
  if (!raw || typeof raw !== 'object') return null

  const record = raw as { version?: unknown; platforms?: unknown }
  if (typeof record.version !== 'string' || record.version.trim() === '') {
    return null
  }
  if (!record.platforms || typeof record.platforms !== 'object') return null

  const version = stripVersionPrefix(record.version)
  const assets: CcSwitchAsset[] = []

  for (const entry of Object.values(
    record.platforms as Record<string, unknown>
  )) {
    const url = (entry as { url?: unknown } | null)?.url
    if (typeof url !== 'string') continue

    const fileName = fileNameFromUrl(url)
    if (!fileName || assets.some((asset) => asset.fileName === fileName)) {
      continue
    }
    assets.push({ fileName, url, mirrorUrl: buildMirrorUrl(version, fileName) })
  }

  if (assets.length === 0) return null

  return { version, source: 'manifest', assets }
}

/**
 * Parse a GitHub Releases API response into a release.
 *
 * The CDN URL is rebuilt from the resolved version, and the GitHub asset URL
 * is kept as the mirror for the same file. Signature sidecars are dropped:
 * they are not installers and only add noise to the catalog.
 */
export function parseCcSwitchGithubRelease(raw: unknown): CcSwitchRelease | null {
  if (!raw || typeof raw !== 'object') return null

  const record = raw as { tag_name?: unknown; assets?: unknown }
  if (typeof record.tag_name !== 'string') return null
  if (!Array.isArray(record.assets)) return null

  const version = stripVersionPrefix(record.tag_name)
  if (version === '') return null

  const assets: CcSwitchAsset[] = []

  for (const entry of record.assets) {
    const name = (entry as { name?: unknown } | null)?.name
    const url = (entry as { browser_download_url?: unknown } | null)
      ?.browser_download_url
    if (typeof name !== 'string' || typeof url !== 'string') continue
    if (name.endsWith('.sig')) continue

    assets.push({
      fileName: name,
      url: buildCdnUrl(version, name),
      mirrorUrl: url,
    })
  }

  if (assets.length === 0) return null

  return { version, source: 'github', assets }
}

/**
 * Find the asset matching a file-name suffix such as `-Windows-arm64.msi`.
 *
 * Matching on a suffix rather than the full name keeps the catalog working if
 * upstream changes the product prefix or the version formatting.
 */
export function findCcSwitchAsset(
  release: CcSwitchRelease,
  fileNameSuffix: string
): CcSwitchAsset | undefined {
  return release.assets.find((asset) =>
    asset.fileName.endsWith(fileNameSuffix)
  )
}

async function fetchJson(
  fetchImpl: typeof fetch,
  url: string,
  timeoutMs: number
): Promise<unknown> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)

  try {
    const response = await fetchImpl(url, { signal: controller.signal })
    if (!response.ok) return null

    return (await response.json()) as unknown
  } catch {
    return null
  } finally {
    clearTimeout(timer)
  }
}

/**
 * Resolve the manifest URL to read, or `null` to skip that source.
 *
 * `dl.ccswitch.io` answers without `Access-Control-Allow-Origin` (verified
 * 2026-09-17), so a cross-origin read is blocked by the browser. The manifest
 * is therefore only used when the deployment serves it same-origin, or when
 * `VITE_CC_SWITCH_MANIFEST_URL` points at a proxy for it.
 */
export function resolveManifestUrl(): string | null {
  const configured = (import.meta.env as Record<string, string | undefined>)
    .VITE_CC_SWITCH_MANIFEST_URL
  if (configured) return configured

  if (typeof window === 'undefined') return null

  try {
    return new URL(CC_SWITCH_MANIFEST_URL).origin === window.location.origin
      ? CC_SWITCH_MANIFEST_URL
      : null
  } catch {
    return null
  }
}

export type CcSwitchResolutionOptions = {
  fetchImpl?: typeof fetch
  /** Manifest URL to try first; `null` (the default) skips it. */
  manifestUrl?: string | null
}

/**
 * Resolve the current CC Switch release, falling back through the sources
 * documented at the top of this module. Never rejects: a failed lookup is
 * reported through `source: 'fallback'` so the page can say so.
 */
export async function resolveCcSwitchRelease(
  options: CcSwitchResolutionOptions = {}
): Promise<CcSwitchRelease> {
  const fetchImpl =
    options.fetchImpl ?? (typeof fetch === 'function' ? fetch : null)
  if (!fetchImpl) return CC_SWITCH_FALLBACK_RELEASE

  const manifestUrl = options.manifestUrl ?? null
  if (manifestUrl) {
    const fromManifest = parseCcSwitchManifest(
      await fetchJson(fetchImpl, manifestUrl, MANIFEST_TIMEOUT_MS)
    )
    if (fromManifest) return fromManifest
  }

  const fromGithub = parseCcSwitchGithubRelease(
    await fetchJson(fetchImpl, CC_SWITCH_RELEASES_API, RELEASES_TIMEOUT_MS)
  )
  if (fromGithub) return fromGithub

  return CC_SWITCH_FALLBACK_RELEASE
}
