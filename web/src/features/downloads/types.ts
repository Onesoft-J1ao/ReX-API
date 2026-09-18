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
 * Desktop platforms this download hub can target.
 *
 * `unknown` is a real outcome (phones, tablets, unusual browsers) and must
 * never be coerced into a guess: an unrecognised visitor simply sees every
 * download without a highlighted recommendation.
 */
export type ClientPlatform = 'windows' | 'macos' | 'linux' | 'unknown'

/** CPU architecture of the visitor, or `unknown` when it cannot be read. */
export type ClientArch = 'x64' | 'arm64' | 'unknown'

/**
 * Architecture of a published build. `universal` marks a single artifact that
 * runs on every architecture of its platform, so it matches any CPU.
 */
export type BuildArch = ClientArch | 'universal'

export type ClientTarget = {
  platform: ClientPlatform
  arch: ClientArch
}

/**
 * Browser signals used for platform detection.
 *
 * Kept as a plain object so detection stays a pure function that can be
 * unit-tested without a DOM. Production callers build it from `navigator`
 * through {@link readDetectionSignals}.
 */
export type DetectionSignals = {
  userAgent?: string | null
  platform?: string | null
  /** `navigator.userAgentData.platform` (User-Agent Client Hint, low entropy). */
  clientHintPlatform?: string | null
  /** `navigator.userAgentData.architecture` (high entropy, Chromium only). */
  clientHintArchitecture?: string | null
}

/** A single downloadable artifact for one platform. */
export type DownloadLink = {
  id: string
  platform: ClientPlatform
  arch: BuildArch
  /** i18n key naming the platform and architecture, e.g. `Windows x64`. */
  platformKey: string
  /** i18n key describing the package itself, e.g. `Windows installer (.msi)`. */
  packageKey: string
  url: string
  /** Alternative host for the same artifact, used when the primary fails. */
  mirrorUrl?: string
}

/** A secondary destination for a tool, shown when direct links break. */
export type BackupLink = {
  /** i18n key, e.g. `Official download page`. */
  labelKey: string
  url: string
}

export type ToolDownload = {
  id: string
  name: string
  /** i18n key describing what the tool is. */
  descriptionKey: string
  homepage: string
  /** Resolved version, or `null` when the tool publishes no machine-readable one. */
  version: string | null
  /** i18n key explaining where {@link version} came from. */
  versionNoteKey: string | null
  links: DownloadLink[]
  backupLinks: BackupLink[]
  checksumsUrl?: string
}

export type ChecksumEntry = {
  fileName: string
  sha256: string
}
