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
  isExtraFile,
  isVideoFile,
  matchesEpisodeNumber,
  selectVideoFile,
} from '../torrent-file'

describe('isVideoFile', () => {
  it('accepts the container formats a release ships in', () => {
    expect(isVideoFile('Show - 01.mkv')).toBe(true)
    expect(isVideoFile('Show - 01.MP4')).toBe(true)
  })

  it('rejects the non-video files a release carries alongside', () => {
    expect(isVideoFile('cover.jpg')).toBe(false)
    expect(isVideoFile('subs.ass')).toBe(false)
  })
})

describe('isExtraFile', () => {
  it('recognises the extras that sit beside the episodes', () => {
    expect(isExtraFile('Show - NCOP.mkv')).toBe(true)
    expect(isExtraFile('Show - OP1.mkv')).toBe(true)
    expect(isExtraFile('Show - PV2.mkv')).toBe(true)
  })

  it('does not mistake an ordinary episode for an extra', () => {
    expect(isExtraFile('Show - 01.mkv')).toBe(false)
  })
})

describe('matchesEpisodeNumber', () => {
  it('matches a bare episode number', () => {
    expect(matchesEpisodeNumber('Show - 03.mkv', 3)).toBe(true)
  })

  it('matches the number after an E marker', () => {
    expect(matchesEpisodeNumber('[Group] Show S01E03 [1080p].mkv', 3)).toBe(true)
  })

  it('matches a Chinese episode marker', () => {
    expect(matchesEpisodeNumber('[Group] Show - 第3话 [1080p].mkv', 3)).toBe(true)
  })

  it('does not match a resolution that happens to contain the number', () => {
    expect(matchesEpisodeNumber('[Group] Show - 1080p.mkv', 1)).toBe(false)
    expect(matchesEpisodeNumber('[Group] Show - 1080p.mkv', 8)).toBe(false)
    expect(matchesEpisodeNumber('[Group] Show - 720p.mkv', 2)).toBe(false)
  })

  it('does not match a longer episode number by its prefix', () => {
    expect(matchesEpisodeNumber('Show - 12.mkv', 1)).toBe(false)
  })
})

describe('selectVideoFile', () => {
  it('returns the only video in the torrent', () => {
    const chosen = selectVideoFile([{ name: 'Show - 01.mkv', length: 100 }])

    expect(chosen?.name).toBe('Show - 01.mkv')
  })

  it('returns null when the torrent carries no video at all', () => {
    expect(
      selectVideoFile([{ name: 'cover.jpg', length: 10 }])
    ).toBeNull()
  })

  it('prefers the file naming the wanted episode over a larger one', () => {
    const chosen = selectVideoFile(
      [
        { name: 'Show - 01.mkv', length: 100 },
        { name: 'Show - 02.mkv', length: 900 },
      ],
      1
    )

    expect(chosen?.name).toBe('Show - 01.mkv')
  })

  it('falls back to the largest video when no file names the episode', () => {
    const chosen = selectVideoFile([
      { name: 'Show - partA.mkv', length: 100 },
      { name: 'Show - partB.mkv', length: 900 },
    ])

    expect(chosen?.name).toBe('Show - partB.mkv')
  })

  it('demotes extras so a season pack plays an episode, not an opening', () => {
    const chosen = selectVideoFile(
      [
        { name: 'Show - NCOP.mkv', length: 999 },
        { name: 'Show - 01.mkv', length: 100 },
      ],
      1
    )

    expect(chosen?.name).toBe('Show - 01.mkv')
  })

  it('still returns an extra when the torrent contains nothing else', () => {
    const chosen = selectVideoFile([{ name: 'Show - NCOP.mkv', length: 999 }])

    expect(chosen?.name).toBe('Show - NCOP.mkv')
  })
})
