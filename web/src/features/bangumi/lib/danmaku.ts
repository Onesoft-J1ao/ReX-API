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
import type { BangumiDanmakuComment } from '../types'

/** One bullet comment, decoded from dandanplay's compact form. */
export type DanmakuComment = {
  id: number
  /** Seconds from the start of the episode. */
  time: number
  /**
   * 1 and 2 scroll right-to-left, 4 sits at the bottom, 5 sits at the top, and
   * 6 scrolls left-to-right. Anything else is treated as a scroll, which is what
   * the overwhelming majority of a real episode is.
   */
  mode: number
  /** A CSS colour, e.g. #ffffff. */
  color: string
  text: string
}

/**
 * Decodes dandanplay's packed colour integer.
 *
 * The value is a 24-bit RGB integer (16777215 is white). Comments on a light
 * video are frequently drawn in near-black, so the renderer keeps the colour
 * rather than forcing one.
 */
export function danmakuColor(value: number): string {
  if (!Number.isFinite(value) || value < 0) return '#ffffff'
  const rgb = Math.floor(value) & 0xffffff
  return `#${rgb.toString(16).padStart(6, '0')}`
}

/**
 * Decodes the comment list.
 *
 * The `p` field is "timeSeconds,mode,color,userHash". A comment that does not
 * carry at least the first four fields cannot be placed on the timeline, and one
 * with no text has nothing to draw, so both are dropped rather than rendered at
 * time zero.
 */
export function parseDanmaku(raw: BangumiDanmakuComment[]): DanmakuComment[] {
  const comments: DanmakuComment[] = []
  for (const entry of raw) {
    if (!entry || typeof entry.p !== 'string' || !entry.m) continue

    const fields = entry.p.split(',')
    if (fields.length < 4) continue

    const time = Number.parseFloat(fields[0])
    const mode = Number.parseInt(fields[1], 10)
    const color = Number.parseInt(fields[2], 10)
    if (!Number.isFinite(time) || time < 0) continue

    comments.push({
      id: entry.cid,
      time,
      mode: Number.isFinite(mode) ? mode : 1,
      color: danmakuColor(color),
      text: entry.m,
    })
  }
  // Sorted once so the renderer can walk a window instead of the whole list on
  // every animation frame.
  comments.sort((a, b) => a.time - b.time)
  return comments
}

/**
 * Splits the viewer's block list into terms.
 *
 * A block list is typed by hand, so it accepts the separators someone is likely
 * to reach for -- a comma, a full-width comma, an enumeration comma or a new
 * line -- and is bounded, because it is a hand-maintained list rather than an
 * input anyone needs to grow without limit.
 */
const BLOCKED_TERM_LIMIT = 200

export function parseBlockedWords(raw: string): string[] {
  const terms = raw
    .split(/[\n,，、]+/)
    .map((term) => term.trim())
    .filter(Boolean)
  return [...new Set(terms)].slice(0, BLOCKED_TERM_LIMIT)
}

/**
 * The comments that survive the viewer's block list.
 *
 * Matching is a case-insensitive substring test rather than a pattern: the list
 * holds words and fragments typed while watching, and a mistyped pattern that
 * silently matches nothing is worse than a broad substring. An empty list keeps
 * everything.
 */
export function filterDanmaku(
  comments: DanmakuComment[],
  blocked: string[]
): DanmakuComment[] {
  const terms = blocked
    .map((term) => term.trim().toLowerCase())
    .filter(Boolean)
  if (terms.length === 0) return comments
  return comments.filter((comment) => {
    const text = comment.text.toLowerCase()
    return !terms.some((term) => text.includes(term))
  })
}

/**
 * How long a comment stays on screen, in seconds.
 *
 * Longer text needs a longer run so it does not cross the screen faster than a
 * short one. The bounds keep a one-character comment from lingering and a very
 * long one from crawling.
 */
export function danmakuDuration(text: string): number {
  const width = Math.max(text.length, 1)
  return Math.min(4 + width * 0.18, 12)
}

/**
 * The comments on screen at `time`.
 *
 * Comments are sorted by time, so a binary search finds the first one that could
 * still be visible and the walk stops as soon as a comment starts after the
 * playhead.
 */
export function commentsInWindow(
  comments: DanmakuComment[],
  time: number,
  maxDuration: number
): DanmakuComment[] {
  const visible: DanmakuComment[] = []
  const earliest = time - maxDuration

  let low = 0
  let high = comments.length
  while (low < high) {
    const mid = (low + high) >> 1
    if (comments[mid].time < earliest) {
      low = mid + 1
    } else {
      high = mid
    }
  }

  for (let i = low; i < comments.length; i += 1) {
    const comment = comments[i]
    if (comment.time > time) break
    if (time - comment.time <= danmakuDuration(comment.text)) {
      visible.push(comment)
    }
  }
  return visible
}
