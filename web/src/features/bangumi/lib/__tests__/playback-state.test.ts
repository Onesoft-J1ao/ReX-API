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

import {
  type PlaybackStatus,
  ZERO_SPEED_TIMEOUT_MS,
  formatBytes,
  isStalled,
  shouldShowOverlay,
} from '../playback-state'

describe('shouldShowOverlay', () => {
  it('covers the video while playback is not running', () => {
    const covered: PlaybackStatus[] = [
      'idle',
      'resolving',
      'connecting',
      'buffering',
      'failed',
    ]

    for (const status of covered) {
      expect(shouldShowOverlay(status)).toBe(true)
    }
  })

  it('uncovers the video once playback is genuinely under way', () => {
    const uncovered: PlaybackStatus[] = ['playing', 'paused', 'ended']

    for (const status of uncovered) {
      expect(shouldShowOverlay(status)).toBe(false)
    }
  })
})

describe('isStalled', () => {
  it('does not report an unknown speed as a stall', () => {
    expect(isStalled(undefined, ZERO_SPEED_TIMEOUT_MS * 2)).toBe(false)
  })

  it('does not report a transfer that is still moving', () => {
    expect(isStalled(1024, ZERO_SPEED_TIMEOUT_MS * 2)).toBe(false)
  })

  it('does not report a zero speed before the timeout elapses', () => {
    expect(isStalled(0, ZERO_SPEED_TIMEOUT_MS - 1)).toBe(false)
  })

  it('reports a zero speed once the timeout elapses', () => {
    expect(isStalled(0, ZERO_SPEED_TIMEOUT_MS)).toBe(true)
  })
})

describe('formatBytes', () => {
  it('treats a missing size as unknown rather than zero', () => {
    expect(formatBytes(0)).toBeNull()
    expect(formatBytes(-1)).toBeNull()
    expect(formatBytes(Number.NaN)).toBeNull()
  })

  it('scales the value to a readable unit', () => {
    expect(formatBytes(500)).toBe('500 B')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
  })

  it('drops the decimal once the value is large enough not to need it', () => {
    expect(formatBytes(150 * 1024)).toBe('150 KB')
  })
})
