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

import { bangumiCoverUrl, bangumiStreamUrl } from '../api'

describe('bangumiStreamUrl', () => {
  it('rewrites a Mikan torrent link onto this instance, carrying the episode', () => {
    expect(
      bangumiStreamUrl(
        'https://mikanani.me/Download/20260721/82591bff.torrent',
        3
      )
    ).toBe('/api/bangumi/stream/Download/20260721/82591bff.torrent?episode=3')
  })

  it('leaves the episode off when it is not known', () => {
    expect(bangumiStreamUrl('http://mikanani.me/Download/a.torrent')).toBe(
      '/api/bangumi/stream/Download/a.torrent'
    )
  })

  it('refuses a link that is not a Mikan download, which the route would reject', () => {
    expect(bangumiStreamUrl('https://mikanani.me/Home/Episode/abc')).toBeNull()
    expect(bangumiStreamUrl('https://example.com/evil.torrent')).toBeNull()
    expect(bangumiStreamUrl('')).toBeNull()
  })
})

describe('bangumiCoverUrl', () => {
  it('sizes the original, because the common thumbnail renders blurred', () => {
    // The calendar ships every size; taking `common` gave a ~3.5 KB thumbnail
    // where the card draws ~200 CSS px, so it was upscaled and looked soft.
    expect(
      bangumiCoverUrl({
        large: 'http://lain.bgm.tv/pic/cover/l/1/2.jpg',
        common: 'http://lain.bgm.tv/pic/cover/c/1/2.jpg',
      })
    ).toBe('/api/bangumi-image/r/400/pic/cover/l/1/2.jpg')
  })

  it('snaps a width the host does not serve up to one it does', () => {
    // The host serves 100/200/400/600/800/1200 and 400s everything else, so a
    // 320 must become a 400 rather than a broken image.
    expect(
      bangumiCoverUrl({ large: 'https://lain.bgm.tv/pic/cover/l/1.jpg' }, 320)
    ).toBe('/api/bangumi-image/r/400/pic/cover/l/1.jpg')
    expect(
      bangumiCoverUrl({ large: 'https://lain.bgm.tv/pic/cover/l/1.jpg' }, 160)
    ).toBe('/api/bangumi-image/r/200/pic/cover/l/1.jpg')
  })

  it('caps a width past the largest the host serves', () => {
    expect(
      bangumiCoverUrl({ large: 'https://lain.bgm.tv/pic/cover/l/1.jpg' }, 4000)
    ).toBe('/api/bangumi-image/r/1200/pic/cover/l/1.jpg')
  })

  it('unwraps the prefix the subject endpoint already added', () => {
    expect(
      bangumiCoverUrl({ large: 'https://lain.bgm.tv/r/400/pic/cover/l/1.jpg' })
    ).toBe('/api/bangumi-image/r/400/pic/cover/l/1.jpg')
  })

  it('does not prefix a thumbnail, which the host rejects with 400', () => {
    // Verified against the host: /r/400/ on a `c` or `m` path returns 400,
    // so a subject with no original keeps its thumbnail rather than a 400.
    expect(
      bangumiCoverUrl({ common: 'http://lain.bgm.tv/pic/cover/c/1/2.jpg' })
    ).toBe('/api/bangumi-image/pic/cover/c/1/2.jpg')
  })

  it('returns null when the subject carries no cover', () => {
    expect(bangumiCoverUrl(undefined)).toBeNull()
    expect(bangumiCoverUrl({})).toBeNull()
  })
})
