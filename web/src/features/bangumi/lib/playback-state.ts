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
 * The playback state model, ported from Animeko's player.
 *
 * Animeko separates a loading state machine (its `VideoLoadingState`) from the
 * engine's own state, and shows its overlay while anything other than a
 * successful load is in effect. The same split is kept here:
 *
 *   idle        -> Initial: nothing chosen yet
 *   resolving   -> ResolvingSource: fetching the torrent, opening the source
 *   connecting  -> DecodingData: querying the swarm for metadata and files
 *   buffering   -> Succeed, but the player has no data to show yet
 *   playing     -> Succeed and the player is advancing
 *   paused/ended-> Succeed, playback stopped
 *   failed      -> Failed, with a reason
 *
 * Animeko's Failed reasons are carried over as-is. `noPeers` is the one addition:
 * Animeko runs a native libtorrent session that connects over TCP/UDP and has no
 * peer-related state or copy at all, whereas a browser reaches a swarm only over
 * WebRTC. A torrent with no WebRTC-capable peers therefore stalls with no data
 * ever arriving, which the visitor deserves to be told plainly rather than
 * watching an endless spinner.
 */
export type PlaybackStatus =
  | 'idle'
  | 'resolving'
  | 'connecting'
  | 'buffering'
  | 'playing'
  | 'paused'
  | 'ended'
  | 'failed'

/** Animeko's `VideoLoadingState.Failed` members, plus `noPeers`. */
export type PlaybackFailure =
  | 'resolutionTimedOut'
  | 'networkError'
  | 'unsupportedMedia'
  | 'noMatchingFile'
  | 'noPeers'
  | 'unknownError'

/**
 * How long the transfer may sit at zero before the viewer is told.
 *
 * Animeko uses exactly 15 seconds of zero speed for the same purpose, and only
 * for BitTorrent: below that it shows a plain buffering state.
 */
export const ZERO_SPEED_TIMEOUT_MS = 15_000

/** The statuses in which playback is under way and the overlay stays hidden. */
const PLAYING_STATUSES = new Set<PlaybackStatus>(['playing', 'paused', 'ended'])

/**
 * Whether the loading/buffering overlay should cover the video.
 *
 * This mirrors Animeko's predicate (`buffering || playerError || state !is
 * Succeed`): the overlay is suppressed only once playback is genuinely running.
 */
export function shouldShowOverlay(status: PlaybackStatus): boolean {
  return !PLAYING_STATUSES.has(status)
}

/**
 * Whether a zero transfer has lasted long enough to report.
 *
 * Speed is in bytes per second; `undefined` means it is not known yet, which
 * must not be reported as zero -- the same distinction Animeko draws between
 * `FileSize.Zero` and `FileSize.Unspecified`.
 */
export function isStalled(
  downloadSpeed: number | undefined,
  stalledForMs: number
): boolean {
  return downloadSpeed === 0 && stalledForMs >= ZERO_SPEED_TIMEOUT_MS
}

/**
 * Formats a byte count for the release list.
 *
 * A size of 0 means the feed did not state one, which is shown as unknown rather
 * than as "0 B".
 */
export function formatBytes(bytes: number): string | null {
  if (!Number.isFinite(bytes) || bytes <= 0) return null
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(value >= 100 || unit === 0 ? 0 : 1)} ${units[unit]}`
}
