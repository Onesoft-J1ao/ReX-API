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
 * The playback state model.
 *
 * The instance relays the video, so the browser's own player is the only thing
 * that knows whether bytes are arriving; these are the states its events map
 * onto. Nothing here samples a transfer, because there is no transfer this side
 * of the relay to sample.
 */
export type PlaybackStatus =
  | 'idle'
  | 'loading'
  | 'playing'
  | 'paused'
  | 'ended'
  | 'failed'

/** The statuses in which playback is under way and the overlay stays hidden. */
const PLAYING_STATUSES = new Set<PlaybackStatus>(['playing', 'paused', 'ended'])

/**
 * Whether the loading or error overlay should cover the video.
 *
 * Suppressed only once playback is genuinely running, so a stall or an error
 * during a stream is visible rather than hidden behind a picture that is not
 * moving.
 */
export function shouldShowOverlay(status: PlaybackStatus): boolean {
  return !PLAYING_STATUSES.has(status)
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
