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
import { beforeEach, describe, expect, it } from 'vitest'

import {
  isFinished,
  readProgress,
  resumePosition,
  writeProgress,
} from '../playback-progress'

beforeEach(() => window.localStorage.clear())

describe('resumePosition', () => {
  const progress = {
    position: 300,
    duration: 1200,
    finished: false,
    updatedAt: 1,
  }

  it('resumes an unfinished episode that is past the opening seconds', () => {
    expect(resumePosition(progress, 1200)).toBe(300)
  })

  it('starts over when the episode was watched through', () => {
    expect(resumePosition({ ...progress, finished: true }, 1200)).toBe(0)
  })

  it('ignores a position a few seconds in as noise', () => {
    expect(resumePosition({ ...progress, position: 3 }, 1200)).toBe(0)
  })

  it('does not seek into the closing credits', () => {
    expect(resumePosition({ ...progress, position: 1180 }, 1200)).toBe(0)
  })

  it('starts over when the duration is unknown', () => {
    expect(resumePosition(progress, Number.NaN)).toBe(0)
    expect(resumePosition(null, 1200)).toBe(0)
  })
})

describe('isFinished', () => {
  it('counts the last half minute as finished', () => {
    expect(isFinished(1180, 1200)).toBe(true)
    expect(isFinished(1000, 1200)).toBe(false)
    expect(isFinished(10, Number.NaN)).toBe(false)
  })
})

describe('progress storage', () => {
  it('round-trips a record under the episode key', () => {
    writeProgress('42:3', {
      position: 120,
      duration: 600,
      finished: false,
      updatedAt: 5,
    })
    expect(readProgress('42:3')).toEqual({
      position: 120,
      duration: 600,
      finished: false,
      updatedAt: 5,
    })
  })

  it('reads back nothing for an unknown or unreadable record', () => {
    expect(readProgress('missing')).toBeNull()
    window.localStorage.setItem('bangumi:progress:broken', '{not json')
    expect(readProgress('broken')).toBeNull()
    window.localStorage.setItem(
      'bangumi:progress:shaped',
      JSON.stringify({ position: 'later', duration: 600 })
    )
    expect(readProgress('shaped')).toBeNull()
  })
})
