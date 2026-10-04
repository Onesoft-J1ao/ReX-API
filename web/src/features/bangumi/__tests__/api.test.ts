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

import { bangumiCoverUrl, bangumiTorrentUrl } from '../api'

describe('bangumiTorrentUrl', () => {
  it('rewrites a Mikan torrent link onto this instance', () => {
    expect(
      bangumiTorrentUrl(
        'https://mikanani.me/Download/20260721/82591bff.torrent'
      )
    ).toBe('/api/bangumi/torrent/Download/20260721/82591bff.torrent')
  })

  it('accepts the plain http form the feed also uses', () => {
    expect(bangumiTorrentUrl('http://mikanani.me/Download/a.torrent')).toBe(
      '/api/bangumi/torrent/Download/a.torrent'
    )
  })

  it('refuses a link that is not a Mikan download, which the proxy would reject', () => {
    expect(bangumiTorrentUrl('https://mikanani.me/Home/Episode/abc')).toBeNull()
    expect(bangumiTorrentUrl('https://example.com/evil.torrent')).toBeNull()
    expect(bangumiTorrentUrl('')).toBeNull()
  })
})

describe('bangumiCoverUrl', () => {
  it('rewrites the calendar cover shape onto the image proxy', () => {
    expect(
      bangumiCoverUrl({ common: 'http://lain.bgm.tv/pic/cover/c/1/2.jpg' })
    ).toBe('/api/bangumi-image/pic/cover/c/1/2.jpg')
  })

  it('keeps the sized prefix the subject endpoint returns', () => {
    expect(
      bangumiCoverUrl(
        { large: 'https://lain.bgm.tv/r/400/pic/cover/l/1.jpg' },
        'large'
      )
    ).toBe('/api/bangumi-image/r/400/pic/cover/l/1.jpg')
  })

  it('returns null when the subject carries no cover', () => {
    expect(bangumiCoverUrl(undefined)).toBeNull()
    expect(bangumiCoverUrl({})).toBeNull()
  })
})
