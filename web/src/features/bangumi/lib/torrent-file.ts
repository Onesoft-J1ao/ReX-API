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
/** The part of a WebTorrent file this module needs. */
export type TorrentFileLike = {
  name: string
  length: number
}

const VIDEO_EXTENSIONS = [
  '.mkv',
  '.mp4',
  '.m4v',
  '.avi',
  '.mov',
  '.webm',
  '.ts',
  '.m2ts',
  '.flv',
  '.wmv',
]

/**
 * Extra material that sits alongside the episodes in the same release.
 *
 * Animeko demotes these rather than dropping them, so a torrent that contains
 * nothing but extras still plays something. The patterns are its blacklist,
 * which matches on the filename.
 */
const EXTRAS_PATTERNS = [
  /\bNC(OP|ED)\b/i,
  /\bOP\d/i,
  /\bED\d/i,
  /\bCM\d/i,
  /\bPV\d?/i,
  /\bSP\d?\b/i,
  /\bOVA\b/i,
  /\bNCOP\b/i,
]

/** Whether a filename looks like a video the browser could play. */
export function isVideoFile(name: string): boolean {
  const lowered = name.toLowerCase()
  return VIDEO_EXTENSIONS.some((extension) => lowered.endsWith(extension))
}

/** Whether a filename is extra material rather than an episode. */
export function isExtraFile(name: string): boolean {
  return EXTRAS_PATTERNS.some((pattern) => pattern.test(name))
}

/**
 * Picks the file to play out of a torrent, following Animeko's
 * `selectVideoFileEntry`.
 *
 * The order is: videos only, extras demoted, a file naming the wanted episode
 * preferred, and the largest remaining video last. A release is a season pack as
 * often as it is a single episode, so "the largest video" alone would play the
 * last episode of a batch when the viewer asked for the first.
 */
export function selectVideoFile<T extends TorrentFileLike>(
  files: T[],
  episodeNumber?: number
): T | null {
  const videos = files.filter((file) => isVideoFile(file.name))
  if (videos.length === 0) return null

  const episodes = videos.filter((file) => !isExtraFile(file.name))
  const pool = episodes.length > 0 ? episodes : videos

  if (episodeNumber != null && Number.isFinite(episodeNumber)) {
    const named = pool.find((file) =>
      matchesEpisodeNumber(file.name, episodeNumber)
    )
    if (named) return named
  }

  // No file names the episode, so take the largest: in a season pack that is a
  // real episode rather than a sample or a menu.
  return pool.reduce((largest, file) =>
    file.length > largest.length ? file : largest
  )
}

/**
 * Whether a filename names the given episode.
 *
 * The number must stand on its own, so "Show 03" and "S01E03" match episode 3
 * while the 1080 of a resolution does not: the trailing lookahead rejects a
 * number that continues into more digits, and the leading class rejects one that
 * continues from them. The leading class is also what makes the E of "S01E03"
 * work -- the marker is simply the non-digit the pattern needs before the
 * number, so no separate E-anchored pattern is required.
 */
export function matchesEpisodeNumber(
  name: string,
  episodeNumber: number
): boolean {
  const escaped = String(episodeNumber)
  const patterns = [
    new RegExp(`(?:^|[^0-9])0*${escaped}(?![0-9])`),
    new RegExp(`第\\s*0*${escaped}\\s*[话話集]`),
  ]
  return patterns.some((pattern) => pattern.test(name))
}
