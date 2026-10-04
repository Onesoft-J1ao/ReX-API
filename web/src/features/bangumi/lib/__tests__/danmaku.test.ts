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
import { describe, expect, it } from 'vitest'

import type { BangumiDanmakuComment } from '../../types'
import {
  commentsInWindow,
  danmakuColor,
  danmakuDuration,
  parseDanmaku,
} from '../danmaku'

function comment(overrides: Partial<BangumiDanmakuComment> = {}) {
  return { cid: 1, p: '10,1,16777215,abc', m: 'hello', ...overrides }
}

describe('danmakuColor', () => {
  it('decodes the packed 24-bit RGB integer', () => {
    expect(danmakuColor(16777215)).toBe('#ffffff')
    expect(danmakuColor(255)).toBe('#0000ff')
    expect(danmakuColor(0)).toBe('#000000')
  })

  it('falls back to white when the value cannot be a colour', () => {
    expect(danmakuColor(-1)).toBe('#ffffff')
    expect(danmakuColor(Number.NaN)).toBe('#ffffff')
  })
})

describe('parseDanmaku', () => {
  it('decodes the packed p field into time, mode, colour and text', () => {
    const parsed = parseDanmaku([
      comment({ p: '12.5,1,16777215,abc', m: '典中典' }),
    ])

    expect(parsed).toEqual([
      { id: 1, time: 12.5, mode: 1, color: '#ffffff', text: '典中典' },
    ])
  })

  it('drops a comment whose p field cannot be placed on the timeline', () => {
    const parsed = parseDanmaku([
      comment({ cid: 1, p: '10,1,16777215' }),
      comment({ cid: 2, p: '10,1,16777215,abc' }),
    ])

    expect(parsed.map((entry) => entry.id)).toEqual([2])
  })

  it('drops a comment with no text or a negative time', () => {
    const parsed = parseDanmaku([
      comment({ cid: 1, m: '' }),
      comment({ cid: 2, p: '-3,1,16777215,abc' }),
      comment({ cid: 3, p: '4,1,16777215,abc' }),
    ])

    expect(parsed.map((entry) => entry.id)).toEqual([3])
  })

  it('sorts by time so the renderer can walk a window', () => {
    const parsed = parseDanmaku([
      comment({ cid: 1, p: '30,1,16777215,a' }),
      comment({ cid: 2, p: '5,1,16777215,a' }),
      comment({ cid: 3, p: '17,1,16777215,a' }),
    ])

    expect(parsed.map((entry) => entry.time)).toEqual([5, 17, 30])
  })

  it('treats an unparseable mode as a scroll rather than dropping the comment', () => {
    const parsed = parseDanmaku([comment({ p: '3,x,16777215,a' })])

    expect(parsed[0].mode).toBe(1)
  })
})

describe('commentsInWindow', () => {
  const comments = parseDanmaku([
    comment({ cid: 1, p: '1,1,16777215,a', m: 'short' }),
    comment({ cid: 2, p: '10,1,16777215,a', m: 'short' }),
    comment({ cid: 3, p: '40,1,16777215,a', m: 'short' }),
  ])

  it('returns the comments that are still on screen at the given time', () => {
    const visible = commentsInWindow(comments, 11, 12)

    expect(visible.map((entry) => entry.id)).toEqual([2])
  })

  it('excludes a comment that has not started yet', () => {
    const visible = commentsInWindow(comments, 5, 12)

    expect(visible.map((entry) => entry.id)).toEqual([1])
  })

  it('returns nothing when no comment is on screen', () => {
    expect(commentsInWindow(comments, 25, 12)).toEqual([])
  })
})

describe('danmakuDuration', () => {
  it('gives longer text a longer run across the screen', () => {
    expect(danmakuDuration('a very long comment indeed')).toBeGreaterThan(
      danmakuDuration('hi')
    )
  })

  it('keeps the duration within bounds for the shortest and longest text', () => {
    expect(danmakuDuration('')).toBeGreaterThan(0)
    expect(danmakuDuration('x'.repeat(500))).toBe(12)
  })
})
