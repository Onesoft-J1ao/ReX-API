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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { BangumiWatchView } from '../components/bangumi-watch-view'
import type { BangumiRelease } from '../types'

const title = 'Anime'
const release: BangumiRelease = {
  topicId: 'release-1',
  title: 'Release one',
  alliance: 'Group',
  description: '',
  downloadLink: 'https://mikanani.me/Download/one.torrent',
  episodeNumber: 1,
  size: 1000,
  publishedAt: '',
  publishedTime: 0,
  resolution: '1080P',
  subtitleLanguages: ['CHS'],
  matchKind: 'EXACT',
  playable: true,
}
const episode = {
  episodeId: 101,
  episodeTitle: 'Episode one',
  episodeNumber: '1',
}
const catalog = {
  sources: [
    {
      id: 'site',
      name: 'Website',
      kind: 'web',
      icon: '',
      tier: 0,
      enabled: true,
    },
    {
      id: 'mikan-mikanime-tv',
      name: '蜜柑计划 (CN)',
      kind: 'bt',
      icon: '',
      tier: 0,
      enabled: true,
    },
  ],
  legacyMikanSourceId: 'mikan-mikanime-tv',
}
let client: QueryClient

beforeEach(() => {
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['bangumi', 'sources'], catalog)
})
afterEach(() => client.clear())

function mount(subjectId = 42) {
  return render(
    <QueryClientProvider client={client}>
      <BangumiWatchView subjectId={subjectId} title={title} onBack={vi.fn()} />
    </QueryClientProvider>
  )
}
function mockWeb() {
  return vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (String(url).endsWith('/episodes')) throw new Error('dandanplay offline')
    if (String(url).endsWith('/search')) {
      return {
        data: {
          sourceId: 'site',
          state: 'ready',
          subjects: [{ id: 'opaque-subject', name: title }],
        },
      }
    }
    if (String(url).includes('/subjects/opaque-subject')) {
      return {
        data: {
          sourceId: 'site',
          state: 'ready',
          channels: [
            {
              id: 'line',
              name: 'Web line',
              label: 'Web line',
              episodes: [
                {
                  id: 'opaque-episode',
                  name: 'Web special',
                  episodeNumber: 'SP',
                },
              ],
            },
          ],
        },
      }
    }
    if (String(url).endsWith('/resolve')) {
      return {
        data: {
          sourceId: 'site',
          state: 'ready',
          streamUrl: '/admitted/web-video',
          format: 'mp4',
        },
      }
    }
    throw new Error(`Unexpected test HTTP request: ${url}`)
  })
}
async function chooseWeb() {
  fireEvent.click(await screen.findByRole('button', { name: 'Website' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Web line' }))
  fireEvent.click(screen.getByRole('button', { name: 'Web special' }))
  const video = await screen.findByLabelText('Anime · Web special', {
    selector: 'video',
  })
  await waitFor(() =>
    expect(video).toHaveAttribute('src', '/admitted/web-video')
  )
  return video
}
async function chooseNative() {
  fireEvent.click(
    await screen.findByRole('tab', { name: /BitTorrent sources/ })
  )
  fireEvent.click(await screen.findByRole('button', { name: '蜜柑计划 (CN)' }))
}

describe('source-aware watch view', () => {
  it('plays the source own episodes when dandanplay fails and never requests unrelated comments', async () => {
    const request = mockWeb()
    mount()
    await chooseWeb()
    // The dandanplay episode list is what failed, so the page degrades to one
    // line next to the player instead of a warning that covers it.
    expect(
      await screen.findByText('Could not load the danmaku episode list.')
    ).toBeVisible()
    expect(screen.queryByText('Could not load the episodes.')).toBeNull()
    expect(
      screen.getByText('Selected source: Website · Web line')
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'No danmaku' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(
      request.mock.calls.some(([url]) => String(url).includes('/danmaku/'))
    ).toBe(false)
    expect(
      request.mock.calls.some(([url]) => String(url).includes('/releases/'))
    ).toBe(false)
  })

  it('picks a playable source by itself so the page opens ready', async () => {
    const request = mockWeb()
    mount()
    // Nothing is clicked: the page probes the source, reaches its episode list
    // and starts playing, which is what makes the dandanplay failure invisible.
    const video = await screen.findByLabelText('Anime · Web special', {
      selector: 'video',
    })
    await waitFor(() =>
      expect(video).toHaveAttribute('src', '/admitted/web-video')
    )
    expect(screen.getByText('Selected source: Website · Web line')).toBeVisible()
    expect(
      screen.queryByText('No source could be started automatically.')
    ).toBeNull()
    expect(
      request.mock.calls.some(([url]) => String(url).includes('/resolve'))
    ).toBe(true)
  })

  it('hands the viewer to the manual picker when nothing can be started automatically', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (String(url).endsWith('/episodes')) throw new Error('dandanplay offline')
      return { data: { sourceId: 'site', state: 'failed', subjects: [] } }
    })
    mount()
    expect(
      await screen.findByText('No source could be started automatically.')
    ).toBeVisible()
    expect(
      screen.getByText(
        'The instance could not reach the media sources. Check its network or proxy.'
      )
    ).toBeVisible()
    // The manual picker is the primary action, and it opens the row rather than
    // only pointing at it.
    fireEvent.click(screen.getByRole('button', { name: 'Choose a source' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Website' })).toHaveAttribute(
        'aria-expanded',
        'true'
      )
    )
  })

  it('moves on to the next source when the picked stream will not decode', async () => {
    client.setQueryData(['bangumi', 'sources'], {
      ...catalog,
      sources: [
        { ...catalog.sources[0], id: 'site', name: 'Website', tier: 0 },
        { ...catalog.sources[0], id: 'backup', name: 'Backup site', tier: 1 },
      ],
    })
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const text = String(url)
      if (text.endsWith('/episodes')) throw new Error('dandanplay offline')
      const id = text.split('/sources/')[1]?.split('/')[0] ?? 'site'
      if (text.endsWith('/search')) {
        return {
          data: {
            sourceId: id,
            state: 'ready',
            subjects: [{ id: `${id}-subject`, name: title }],
          },
        }
      }
      if (text.includes('/subjects/')) {
        return {
          data: {
            sourceId: id,
            state: 'ready',
            channels: [
              {
                id: `${id}-line`,
                name: `${id} line`,
                label: `${id} line`,
                episodes: [
                  { id: `${id}-episode`, name: 'Episode one', episodeNumber: '1' },
                ],
              },
            ],
          },
        }
      }
      return {
        data: {
          sourceId: id,
          state: 'ready',
          // The first source resolves but hands back a stream the browser
          // cannot decode, which is what the fallback is for.
          streamUrl: id === 'site' ? '/admitted/broken' : '/admitted/good',
          format: 'mp4',
        },
      }
    })
    mount()
    const first = await screen.findByLabelText('Anime · Episode one', {
      selector: 'video',
    })
    await waitFor(() =>
      expect(first).toHaveAttribute('src', '/admitted/broken')
    )
    fireEvent.error(first)
    await waitFor(() =>
      expect(
        screen.getByLabelText('Anime · Episode one', { selector: 'video' })
      ).toHaveAttribute('src', '/admitted/good')
    )
    expect(
      screen.getByText('Selected source: Backup site · backup line')
    ).toBeVisible()
  })

  it('names a browser check rather than the network when that is what blocked every source', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (String(url).endsWith('/episodes')) throw new Error('dandanplay offline')
      return {
        data: {
          sourceId: 'site',
          state: 'captcha_required',
          captchaKind: 'cloudflare',
          subjects: [],
        },
      }
    })
    mount()
    expect(
      await screen.findByText(
        'Every source it tried needs a check only a browser can pass. Pick one below and clear it.'
      )
    ).toBeVisible()
  })

  it('loads comments only for an explicitly selected known dandanplay episode', async () => {
    client.setQueryData(['bangumi', 'episodes', 42], {
      animeId: 42,
      animeTitle: title,
      episodes: [episode],
    })
    const request = mockWeb()
    request.mockImplementation(async (url) => {
      if (String(url).includes('/danmaku/')) {
        return { data: { count: 0, comments: [] } }
      }
      if (String(url).endsWith('/search')) {
        return {
          data: {
            sourceId: 'site',
            state: 'ready',
            subjects: [{ id: 'opaque-subject', name: title }],
          },
        }
      }
      if (String(url).includes('/subjects/opaque-subject')) {
        return {
          data: {
            sourceId: 'site',
            state: 'ready',
            channels: [
              {
                id: 'line',
                name: 'Web line',
                label: 'Web line',
                episodes: [
                  {
                    id: 'opaque-episode',
                    name: 'Web special',
                    episodeNumber: 'SP',
                  },
                ],
              },
            ],
          },
        }
      }
      return {
        data: {
          sourceId: 'site',
          state: 'ready',
          streamUrl: '/admitted/web-video',
          format: 'mp4',
        },
      }
    })
    mount()
    await chooseWeb()
    expect(
      request.mock.calls.some(([url]) => String(url).includes('/danmaku/'))
    ).toBe(false)
    fireEvent.click(
      screen.getByRole('button', { name: 'Use danmaku for Episode one' })
    )
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        '/api/bangumi/danmaku/101',
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      )
    )
  })

  it('does not fall back to the first Mikan release when no release is mapped to the selected episode', async () => {
    client.setQueryData(['bangumi', 'episodes', 42], {
      animeId: 42,
      animeTitle: title,
      episodes: [episode],
    })
    client.setQueryData(['bangumi', 'releases', 42, title], {
      subjectId: 42,
      releases: [release],
      episodes: [episode],
    })
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { sourceId: 'site', state: 'empty', subjects: [] },
    })
    mount()
    await chooseNative()
    const video = screen.getByLabelText('Episode one', { selector: 'video' })
    expect(video).not.toHaveAttribute('src')
    expect(screen.getByText('Pick an episode to start watching.')).toBeVisible()
    fireEvent.click(await screen.findByRole('button', { name: 'Release one' }))
    await waitFor(() =>
      expect(video).toHaveAttribute(
        'src',
        '/api/bangumi/stream/Download/one.torrent?episode=1'
      )
    )
  })

  it('preserves native Mikan episode mappings without querying the generic native source manager', async () => {
    client.setQueryData(['bangumi', 'episodes', 42], {
      animeId: 42,
      animeTitle: title,
      episodes: [episode],
    })
    client.setQueryData(['bangumi', 'releases', 42, title], {
      subjectId: 42,
      releases: [release],
      episodes: [{ ...episode, release }],
    })
    client.setQueryData(['bangumi', 'danmaku', 101], { count: 0, comments: [] })
    const request = vi.spyOn(api, 'get').mockResolvedValue({
      data: { sourceId: 'site', state: 'empty', subjects: [] },
    })
    mount()
    await chooseNative()
    await waitFor(() =>
      expect(
        screen.getByLabelText('Episode one', { selector: 'video' })
      ).toHaveAttribute(
        'src',
        '/api/bangumi/stream/Download/one.torrent?episode=1'
      )
    )
    expect(
      request.mock.calls.some(([url]) =>
        String(url).includes('/mikan-mikanime-tv/')
      )
    ).toBe(false)
  })

  it('does not stream an unknown Mikan season pack when no episode number is known', async () => {
    client.setQueryData(['bangumi', 'episodes', 42], {
      animeId: 42,
      animeTitle: title,
      episodes: [],
    })
    client.setQueryData(['bangumi', 'releases', 42, title], {
      subjectId: 42,
      releases: [{ ...release, episodeNumber: 0 }],
      episodes: [],
    })
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { sourceId: 'site', state: 'empty', subjects: [] },
    })
    mount()
    await chooseNative()
    fireEvent.click(await screen.findByRole('button', { name: 'Release one' }))
    expect(
      await screen.findByText(
        'Select a known episode before playing a season pack.'
      )
    ).toBeVisible()
    expect(document.querySelector('video')).not.toHaveAttribute('src')
  })

  it('shares the two-request limit with native Mikan and cancels native lookup when selecting another source', async () => {
    client.setQueryData(['bangumi', 'episodes', 42], {
      animeId: 42,
      animeTitle: title,
      episodes: [],
    })
    client.setQueryData(['bangumi', 'sources'], {
      ...catalog,
      sources: [
        catalog.sources[1],
        ...[1, 2, 3].map((number) => ({
          id: `bt-${number}`,
          name: `BT source ${number}`,
          kind: 'bt',
          icon: '',
          tier: 0,
          enabled: true,
        })),
      ],
    })
    const pending: {
      url: string
      signal: AbortSignal
      resolve: (value: { data: object }) => void
    }[] = []
    vi.spyOn(api, 'get').mockImplementation(
      (url, config) =>
        new Promise((resolve, reject) => {
          const signal = config?.signal as AbortSignal
          signal.addEventListener(
            'abort',
            () => reject(new DOMException('Cancelled', 'AbortError')),
            { once: true }
          )
          pending.push({ url: String(url), signal, resolve })
        })
    )
    mount()
    await chooseNative()
    await waitFor(() => expect(pending).toHaveLength(2))
    expect(pending.some((item) => item.url.includes('/releases/'))).toBe(false)
    pending[0].resolve({
      data: { sourceId: 'bt-1', state: 'empty', subjects: [] },
    })
    await waitFor(() => expect(pending).toHaveLength(3))
    expect(pending[2].url).toBe('/api/bangumi/releases/42')
    fireEvent.click(screen.getByRole('button', { name: 'BT source 2' }))
    await waitFor(() => expect(pending[2].signal.aborted).toBe(true))
  })

  it('detaches the video when the session unmounts and does not leak playback across subject changes', async () => {
    mockWeb()
    const view = mount()
    const oldVideo = await chooseWeb()
    // The player is a page now, so "closing" is unmounting it. The session is
    // keyed by subject, so a different subject is a fresh mount rather than a
    // re-render, which is what the old dialog's open toggle used to exercise.
    view.unmount()
    expect(oldVideo).not.toHaveAttribute('src')

    mount(43)
    expect(
      await screen.findByRole('heading', { name: title })
    ).toBeInTheDocument()
    // The new subject runs its own pick rather than inheriting the old
    // session's playback, which is what the detached element above proves.
    await waitFor(() =>
      expect(
        screen.getByLabelText('Anime · Web special', { selector: 'video' })
      ).toHaveAttribute('src', '/admitted/web-video')
    )
  })
})
