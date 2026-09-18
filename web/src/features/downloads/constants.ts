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
  findCcSwitchAsset,
  type CcSwitchRelease,
  CC_SWITCH_RELEASES_URL,
} from './lib/cc-switch'
import { CLAUDE_CHECKSUMS_URL, CLAUDE_DOWNLOAD_BASE } from './lib/checksums'
import type {
  BuildArch,
  ClientPlatform,
  DownloadLink,
  ToolDownload,
} from './types'

/** Official upstream of the mirrored Claude desktop builds. */
export const CLAUDE_OFFICIAL_URL = 'https://claude.ai/download'

export const CC_SWITCH_HOMEPAGE = 'https://ccswitch.io'

/** Platforms a visitor can pick from when auto-detection is wrong. */
export const SELECTABLE_PLATFORMS: ClientPlatform[] = [
  'windows',
  'macos',
  'linux',
]

export const PLATFORM_LABEL_KEYS: Record<ClientPlatform, string> = {
  windows: 'Windows',
  macos: 'macOS',
  linux: 'Linux',
  unknown: 'Unknown',
}

type CcSwitchSlot = {
  id: string
  /** Suffix of the published file name, matched against the resolved release. */
  fileNameSuffix: string
  platform: ClientPlatform
  arch: BuildArch
  platformKey: string
  packageKey: string
}

/**
 * Artifacts published for CC Switch.
 *
 * Slots are matched against the resolved release by file-name suffix, and a
 * slot with no matching asset is dropped instead of rendered as a dead link.
 */
const CC_SWITCH_SLOTS: CcSwitchSlot[] = [
  {
    id: 'windows-x64',
    fileNameSuffix: '-Windows.msi',
    platform: 'windows',
    arch: 'x64',
    platformKey: 'Windows x64',
    packageKey: 'Windows installer (.msi)',
  },
  {
    id: 'windows-arm64',
    fileNameSuffix: '-Windows-arm64.msi',
    platform: 'windows',
    arch: 'arm64',
    platformKey: 'Windows arm64',
    packageKey: 'Windows ARM64 installer (.msi)',
  },
  {
    id: 'macos',
    fileNameSuffix: '-macOS.dmg',
    platform: 'macos',
    arch: 'universal',
    platformKey: 'macOS (universal)',
    packageKey: 'macOS disk image (.dmg)',
  },
  {
    id: 'linux-x64',
    fileNameSuffix: '-Linux-x86_64.AppImage',
    platform: 'linux',
    arch: 'x64',
    platformKey: 'Linux x64',
    packageKey: 'Linux AppImage (.AppImage)',
  },
]

/**
 * Claude desktop builds published by the mirror.
 *
 * These are the fixed `/latest/…` channel endpoints, so the URLs never carry
 * a version and need no resolution step.
 */
const CLAUDE_LINKS: DownloadLink[] = [
  {
    id: 'windows-x64',
    platform: 'windows',
    arch: 'x64',
    platformKey: 'Windows x64',
    packageKey: 'Windows package (.msix)',
    url: `${CLAUDE_DOWNLOAD_BASE}/win-x64`,
  },
  {
    id: 'windows-arm64',
    platform: 'windows',
    arch: 'arm64',
    platformKey: 'Windows arm64',
    packageKey: 'Windows package (.msix)',
    url: `${CLAUDE_DOWNLOAD_BASE}/win-arm64`,
  },
  {
    id: 'macos',
    platform: 'macos',
    arch: 'universal',
    platformKey: 'macOS (universal)',
    packageKey: 'macOS universal disk image (.dmg)',
    url: `${CLAUDE_DOWNLOAD_BASE}/mac`,
  },
]

function buildCcSwitchLinks(release: CcSwitchRelease): DownloadLink[] {
  const links: DownloadLink[] = []

  for (const slot of CC_SWITCH_SLOTS) {
    const asset = findCcSwitchAsset(release, slot.fileNameSuffix)
    if (!asset) continue

    links.push({
      id: slot.id,
      platform: slot.platform,
      arch: slot.arch,
      platformKey: slot.platformKey,
      packageKey: slot.packageKey,
      url: asset.url,
      mirrorUrl: asset.mirrorUrl,
    })
  }

  return links
}

function buildCcSwitchTool(
  release: CcSwitchRelease,
  resolving: boolean
): ToolDownload {
  let versionNoteKey = 'Live version lookup failed, showing the pinned version {{version}}.'
  if (resolving) {
    versionNoteKey = 'Resolving the latest version…'
  } else if (release.source !== 'fallback') {
    versionNoteKey = 'Version resolved from the upstream release channel.'
  }

  return {
    id: 'cc-switch',
    name: 'CC Switch',
    descriptionKey:
      'Switch between multiple Claude Code and Codex provider configurations from a desktop tray app.',
    homepage: CC_SWITCH_HOMEPAGE,
    version: release.version,
    versionNoteKey,
    links: buildCcSwitchLinks(release),
    backupLinks: [
      { labelKey: 'GitHub releases', url: CC_SWITCH_RELEASES_URL },
    ],
  }
}

function buildClaudeCodexTool(): ToolDownload {
  return {
    id: 'claude-codex',
    name: 'Claude Codex',
    descriptionKey:
      'Claude desktop build for Windows and macOS, published as a Microsoft Store package.',
    homepage: CLAUDE_OFFICIAL_URL,
    version: null,
    versionNoteKey: null,
    links: CLAUDE_LINKS,
    backupLinks: [
      { labelKey: 'Official download page', url: CLAUDE_OFFICIAL_URL },
    ],
    checksumsUrl: CLAUDE_CHECKSUMS_URL,
  }
}

export function buildDownloadCatalog(
  release: CcSwitchRelease,
  options: { resolving?: boolean } = {}
): ToolDownload[] {
  return [
    buildCcSwitchTool(release, options.resolving ?? false),
    buildClaudeCodexTool(),
  ]
}
