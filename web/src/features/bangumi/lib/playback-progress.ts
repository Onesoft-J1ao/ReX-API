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
 * Remembering where an episode was left.
 *
 * The video is relayed by this instance and the browser keeps no history of its
 * own, so the position is held in the viewer's own storage: it is their private
 * viewing position, not server state, and it must survive a reload of a page
 * whose media URL is regenerated on every resolve.
 */
export type PlaybackProgress = {
  /** Seconds into the episode. */
  position: number
  duration: number
  /**
   * Whether the episode ran to its end. A finished record is kept -- the viewer
   * may replay the episode -- but resuming it starts from the beginning.
   */
  finished: boolean
  updatedAt: number
}

/** Resuming a few seconds in is noise, so the position has to be worth having. */
const MIN_RESUME_SECONDS = 5
/**
 * How close to the end counts as finished. Landing in the credits is worse than
 * starting over, and the credits are what the last half minute is.
 */
const END_GUARD_SECONDS = 30

const STORAGE_PREFIX = 'bangumi:progress:'

function storage(): Storage | null {
  // Private-mode browsers and a blocked third-party context both throw on
  // access, and neither is a reason to break playback.
  try {
    return window.localStorage
  } catch {
    return null
  }
}

/** Whether a position is close enough to the end to count as watched through. */
export function isFinished(position: number, duration: number): boolean {
  if (!Number.isFinite(duration) || duration <= 0) return false
  return duration - position < END_GUARD_SECONDS
}

/**
 * Where to start playback, in seconds.
 *
 * Zero unless the record is worth resuming: an unfinished episode, past the
 * opening seconds, and not already in the closing credits.
 */
export function resumePosition(
  progress: PlaybackProgress | null,
  duration: number
): number {
  if (!progress || progress.finished) return 0
  if (!Number.isFinite(duration) || duration <= 0) return 0
  if (progress.position < MIN_RESUME_SECONDS) return 0
  if (isFinished(progress.position, duration)) return 0
  return progress.position
}

export function readProgress(key: string): PlaybackProgress | null {
  const store = storage()
  if (!store) return null
  try {
    const raw = store.getItem(STORAGE_PREFIX + key)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PlaybackProgress>
    if (
      typeof parsed.position !== 'number' ||
      typeof parsed.duration !== 'number' ||
      !Number.isFinite(parsed.position) ||
      parsed.position < 0
    ) {
      return null
    }
    return {
      position: parsed.position,
      duration: Number.isFinite(parsed.duration) ? parsed.duration : 0,
      finished: parsed.finished === true,
      updatedAt: typeof parsed.updatedAt === 'number' ? parsed.updatedAt : 0,
    }
  } catch {
    return null
  }
}

export function writeProgress(key: string, progress: PlaybackProgress): void {
  const store = storage()
  if (!store) return
  try {
    store.setItem(STORAGE_PREFIX + key, JSON.stringify(progress))
  } catch {
    // A full or disabled storage loses the position; playback continues.
  }
}
