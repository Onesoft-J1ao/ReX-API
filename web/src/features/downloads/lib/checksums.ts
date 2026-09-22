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
import type { ChecksumEntry } from '../types'

export const CLAUDE_DOWNLOAD_BASE = 'https://claudeapp.agentsmirror.com/latest'
export const CLAUDE_CHECKSUMS_URL = `${CLAUDE_DOWNLOAD_BASE}/checksums`

const CHECKSUMS_TIMEOUT_MS = 6000

const SHA256SUM_LINE = /^([0-9a-f]{64})\s+\*?(.+?)\s*$/i
const BSD_LINE = /^SHA256\s*\((.+)\)\s*=\s*([0-9a-f]{64})\s*$/i

/**
 * Parse a `sha256sum`-style manifest.
 *
 * Both the GNU format (`<hash>  <file>`, with an optional `*` binary marker)
 * and the BSD format (`SHA256 (file) = <hash>`) are accepted, because the
 * published manifest is an external contract that may change format.
 */
export function parseChecksumManifest(text: string): ChecksumEntry[] {
  const entries: ChecksumEntry[] = []

  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (line === '' || line.startsWith('#')) continue

    const bsd = BSD_LINE.exec(line)
    if (bsd) {
      entries.push({ fileName: bsd[1].trim(), sha256: bsd[2].toLowerCase() })
      continue
    }

    const gnu = SHA256SUM_LINE.exec(line)
    if (gnu) {
      entries.push({ fileName: gnu[2].trim(), sha256: gnu[1].toLowerCase() })
    }
  }

  return entries
}

/**
 * Last known checksums, captured from the published manifest on
 * {@link CLAUDE_CHECKSUMS_SNAPSHOT_DATE}.
 *
 * The mirror serves no `Access-Control-Allow-Origin`, so a browser read of
 * the live file is normally blocked. This snapshot keeps the dialog useful in
 * that case, and it is always labelled as a snapshot so it is never mistaken
 * for the current values.
 */
export const CLAUDE_CHECKSUMS_SNAPSHOT_DATE = '2026-09-17'

export const CLAUDE_CHECKSUMS_SNAPSHOT: ChecksumEntry[] = [
  {
    fileName: 'Claude-mac-universal.dmg',
    sha256:
      'a1d7ec9c231ea10f71562f3e1fd48dcc4cba415dfc045131e55bdb23d4d2b373',
  },
  {
    fileName: 'Claude-win-arm64.msix',
    sha256:
      'a48cb4c7e7049dfc89578a1d391d3fe34e5f3b7fba088119d91e603f0ac7da30',
  },
  {
    fileName: 'Claude-win-x64.msix',
    sha256:
      'bc39cbe4bc26a9235737535232f5fba84875bd5f438994505b8ab123a8ef7cb9',
  },
  {
    fileName: 'release-manifest.json',
    sha256:
      '592433a275874df62ae77c64d2a4b4dabf7117ad0a765601c5a5e676c9f2e9d1',
  },
]

export type ChecksumSnapshot = {
  entries: ChecksumEntry[]
  /** `true` when the entries were read from the mirror in this session. */
  live: boolean
  /** Date the bundled entries were captured; `null` for live reads. */
  snapshotDate: string | null
}

export function bundledChecksumSnapshot(): ChecksumSnapshot {
  return {
    entries: CLAUDE_CHECKSUMS_SNAPSHOT,
    live: false,
    snapshotDate: CLAUDE_CHECKSUMS_SNAPSHOT_DATE,
  }
}

/**
 * Read the published checksums, degrading to the bundled snapshot.
 *
 * Never rejects: the caller renders either the live values or the snapshot,
 * and the dialog states which one it is showing.
 */
export async function fetchClaudeChecksums(
  fetchImpl?: typeof fetch
): Promise<ChecksumSnapshot> {
  const doFetch = fetchImpl ?? (typeof fetch === 'function' ? fetch : null)
  if (!doFetch) return bundledChecksumSnapshot()

  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), CHECKSUMS_TIMEOUT_MS)

  try {
    const response = await doFetch(CLAUDE_CHECKSUMS_URL, {
      signal: controller.signal,
    })
    if (!response.ok) return bundledChecksumSnapshot()

    const entries = parseChecksumManifest(await response.text())
    if (entries.length === 0) return bundledChecksumSnapshot()

    return { entries, live: true, snapshotDate: null }
  } catch {
    return bundledChecksumSnapshot()
  } finally {
    clearTimeout(timer)
  }
}
