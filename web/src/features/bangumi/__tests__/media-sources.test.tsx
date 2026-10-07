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
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { BangumiMediaSources } from '../components/bangumi-media-sources'
import type { BangumiMediaSource, BangumiSourceState } from '../types'

const names = [
  '稀饭动漫',
  'girigiri愛動漫',
  '嘀嗒影视',
  '叽哔动漫',
  'E-ACG',
  '森之屋动漫',
  '风车影视',
  '去看吧',
  '海星动漫',
  '樱花动漫',
  '嘀哩嘀哩',
  '新优酷',
  '番茄动漫',
  'hanime1_1080p',
  'hanime1_720p',
  'wedm',
  '影视森林',
  '热播之家',
]
const webSources: BangumiMediaSource[] = names.map((name, index) => ({
  id: `web-${index}`,
  name,
  kind: 'web',
  icon: 'https://source.example/icon.png',
  tier: index,
  enabled: true,
  websiteUrl: `https://source.example/${index}`,
}))
const btSources: BangumiMediaSource[] = [
  '蜜柑计划 (CN)',
  '動漫花園',
  'nyaa.land',
  'AnimeGarden',
].map((name, index) => ({
  id: index === 0 ? 'mikan-mikanime-tv' : `bt-${index}`,
  name,
  kind: 'bt',
  icon: '',
  tier: 0,
  enabled: true,
}))
let client: QueryClient
const play = vi.fn()
const native = vi.fn()

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  play.mockReset()
  native.mockReset()
})
afterEach(() => client.clear())

function mount(sources = webSources, active = true) {
  client.setQueryData(['bangumi', 'sources'], {
    sources,
    legacyMikanSourceId: 'mikan-mikanime-tv',
  })
  const props = {
    keyword: 'Anime',
    active,
    onPlay: play,
    onSelectionChange: vi.fn(),
    onNativeSelect: native,
    nativeState: 'idle' as const,
    nativeRequest: async () => ({
      subjectId: 42,
      matchKind: 'EXACT',
      releases: [],
      episodes: [],
    }),
    nativeMikan: <span>Native releases</span>,
  }
  const view = render(
    <QueryClientProvider client={client}>
      <BangumiMediaSources {...props} />
    </QueryClientProvider>
  )
  return { ...view, props }
}

function searchReady(
  id: string,
  subjects = [{ id: 'subject', name: 'Anime' }]
) {
  return { data: { sourceId: id, state: 'ready', subjects } }
}
function lines(id = 'web-0') {
  return {
    data: {
      sourceId: id,
      state: 'ready',
      channels: [
        {
          id: 'line-a',
          name: 'Line A',
          label: 'Same label',
          episodes: [
            { id: 'episode/a', name: 'Decimal episode', episodeNumber: '12.5' },
          ],
        },
        {
          id: 'line-b',
          name: 'Line B',
          label: 'Same label',
          episodes: [
            { id: 'episode/b', name: 'Special episode', episodeNumber: 'SP' },
            {
              id: 'episode/c',
              name: 'Chinese episode',
              episodeNumber: '第十二话',
            },
          ],
        },
      ],
    },
  }
}

async function openFirst() {
  fireEvent.click(await screen.findByRole('button', { name: names[0] }))
  await screen.findAllByRole('button', { name: 'Same label' })
}

describe('media source selection', () => {
  it('renders all 22 real source names with independent status and native Mikan routing', async () => {
    const states: BangumiSourceState[] = [
      'empty',
      'captcha_required',
      'rate_limited',
      'browser_required',
      'invalid_config',
      'failed',
    ]
    const request = vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const id = String(url).split('/')[4]
      const state = states[Number(id?.split('-')[1]) % states.length]
      return {
        data: {
          sourceId: id,
          state,
          subjects: [],
          retryAt: '2030-01-01T00:00:00Z',
        },
      }
    })
    mount([...btSources, ...webSources])
    expect(
      screen.getByRole('tab', { name: /Website sources.*18/ })
    ).toHaveAttribute('aria-selected', 'true')
    for (const name of names) {
      expect(screen.getByRole('button', { name })).toBeVisible()
    }
    await waitFor(() =>
      expect(screen.getAllByText('Invalid source configuration')).toHaveLength(
        3
      )
    )
    expect(screen.getAllByText('CAPTCHA required')[0]).toHaveClass(
      'text-destructive'
    )
    expect(screen.getAllByText('Browser required')).toHaveLength(3)
    expect(screen.getAllByText('Source rate limited')).toHaveLength(3)
    expect(document.querySelector('img')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: names[1] }))
    // A challenge is not a dead end: the picker links out to the source's own
    // site so the viewer can solve it, and collects the cookie it sets.
    expect(
      await screen.findByText(
        'This source is asking the server for a check only a browser can pass. Open the site, finish the check there, then paste the cookie it sets to continue.'
      )
    ).toBeVisible()
    expect(screen.getByRole('link', { name: `Open ${names[1]}` })).toHaveAttribute(
      'href',
      'https://source.example/1'
    )
    const clearance = screen.getByLabelText('Clearance cookie')
    const verify = screen.getByRole('button', { name: 'Verify and retry' })
    expect(verify).toBeDisabled()
    fireEvent.change(clearance, { target: { value: 'cf_clearance=abc' } })
    expect(verify).toBeEnabled()
    fireEvent.click(verify)
    await waitFor(() =>
      expect(
        request.mock.calls.some(([, config]) =>
          String(config?.params?.clearance ?? '').includes('cf_clearance=abc')
        )
      ).toBe(true)
    )
    expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible()
    fireEvent.click(screen.getByRole('tab', { name: /BitTorrent sources.*4/ }))
    for (const source of btSources) {
      expect(
        await screen.findByRole('button', { name: source.name })
      ).toBeVisible()
    }
    fireEvent.click(screen.getByRole('button', { name: btSources[0].name }))
    expect(await screen.findByText('Native releases')).toBeVisible()
    expect(native).toHaveBeenCalledOnce()
    expect(
      request.mock.calls.some(([url]) =>
        String(url).includes('/mikan-mikanime-tv/')
      )
    ).toBe(false)
  })

  it('keeps duplicate line labels distinct and preserves decimal, special and Chinese episode labels', async () => {
    const request = vi
      .spyOn(api, 'get')
      .mockImplementation(async (url, config) => {
        if (String(url).endsWith('/search')) return searchReady('web-0')
        if (String(url).includes('/subjects/')) return lines()
        return {
          data: {
            sourceId: 'web-0',
            state: 'ready',
            streamUrl: '/admitted/video',
            format: 'mp4',
            requestedEpisode: config?.params.episodeId,
          },
        }
      })
    mount([webSources[0]])
    await openFirst()
    const chips = screen.getAllByRole('button', { name: 'Same label' })
    fireEvent.click(chips[0])
    expect(chips[0]).toHaveAttribute('aria-pressed', 'true')
    expect(
      screen.getByRole('button', { name: 'Decimal episode' })
    ).toHaveTextContent('12.5')
    fireEvent.click(chips[1])
    expect(chips[0]).toHaveAttribute('aria-pressed', 'false')
    expect(
      screen.queryByRole('button', { name: 'Decimal episode' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Chinese episode' })
    ).toHaveTextContent('第十二话')
    fireEvent.click(screen.getByRole('button', { name: 'Special episode' }))
    await waitFor(() =>
      expect(play).toHaveBeenCalledWith(
        expect.objectContaining({
          channelId: 'line-b',
          episode: expect.objectContaining({
            id: 'episode/b',
            episodeNumber: 'SP',
          }),
          streamUrl: '/admitted/video',
          format: 'mp4',
        })
      )
    )
    expect(request).toHaveBeenCalledWith(
      '/api/bangumi/sources/web-0/resolve',
      expect.objectContaining({
        params: { episodeId: 'episode/b' },
        signal: expect.any(AbortSignal),
      })
    )
  })

  it('does not browse the first of multiple subject candidates until the visitor chooses one', async () => {
    const request = vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (String(url).endsWith('/search')) {
        return searchReady('web-0', [
          { id: 'first', name: 'Season one' },
          { id: 'second/opaque', name: 'Season two' },
        ])
      }
      return lines()
    })
    mount([webSources[0]])
    fireEvent.click(await screen.findByRole('button', { name: names[0] }))
    fireEvent.click(await screen.findByRole('button', { name: 'Season two' }))
    expect(
      await screen.findAllByRole('button', { name: 'Same label' })
    ).toHaveLength(2)
    expect(
      request.mock.calls
        .filter(([url]) => String(url).includes('/subjects/'))
        .map(([url]) => url)
    ).toEqual(['/api/bangumi/sources/web-0/subjects/second%2Fopaque'])
  })

  it('shows a retryable failure for one site without hiding successful lines on another', async () => {
    let offline = true
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (String(url).includes('/web-0/')) {
        if (offline) throw new Error('Offline')
        if (String(url).endsWith('/search')) return searchReady('web-0')
        return lines()
      }
      if (String(url).endsWith('/search')) return searchReady('web-1')
      return lines('web-1')
    })
    mount(webSources.slice(0, 2))
    await screen.findByText('Source query failed')
    fireEvent.click(screen.getByRole('button', { name: names[1] }))
    expect(
      await screen.findAllByRole('button', { name: 'Same label' })
    ).toHaveLength(2)
    fireEvent.click(screen.getByRole('button', { name: names[0] }))
    expect(await screen.findByText('Could not load this source')).toBeVisible()
    offline = false
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findAllByRole('button', { name: 'Same label' })
    ).toHaveLength(2)
  })

  it('limits actual source HTTP queries to two and aborts both network and queued work on close', async () => {
    const pending: {
      signal: AbortSignal
      resolve: (value: { data: object }) => void
    }[] = []
    vi.spyOn(api, 'get').mockImplementation(
      (_url, config) =>
        new Promise((resolve) =>
          pending.push({ signal: config?.signal as AbortSignal, resolve })
        )
    )
    const view = mount()
    await waitFor(() => expect(pending).toHaveLength(2))
    expect(screen.getAllByText('Searching source…')).toHaveLength(18)
    view.rerender(
      <QueryClientProvider client={client}>
        <BangumiMediaSources {...view.props} active={false} />
      </QueryClientProvider>
    )
    await waitFor(() =>
      expect(pending.every((item) => item.signal.aborted)).toBe(true)
    )
    await act(async () => {
      for (const item of pending) {
        item.resolve({
          data: { sourceId: 'web-0', state: 'empty', subjects: [] },
        })
      }
    })
    expect(pending).toHaveLength(2)
    expect(
      screen.queryByRole('button', { name: names[0] })
    ).not.toBeInTheDocument()
  })

  it('cancels a stale keyword queue before starting the new search', async () => {
    const pending: {
      keyword: string
      signal: AbortSignal
      resolve: (value: { data: object }) => void
    }[] = []
    vi.spyOn(api, 'get').mockImplementation(
      (_url, config) =>
        new Promise((resolve, reject) => {
          const signal = config?.signal as AbortSignal
          signal.addEventListener(
            'abort',
            () => reject(new DOMException('Cancelled', 'AbortError')),
            { once: true }
          )
          pending.push({ keyword: config?.params.keyword, signal, resolve })
        })
    )
    mount()
    await waitFor(() => expect(pending).toHaveLength(2))
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Search source titles' }),
      { target: { value: 'New season' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    await waitFor(() =>
      expect(
        pending.filter((item) => item.keyword === 'New season')
      ).toHaveLength(2)
    )
    expect(
      pending
        .filter((item) => item.keyword === 'Anime')
        .every((item) => item.signal.aborted)
    ).toBe(true)
    expect(pending.filter((item) => item.keyword === 'Anime')).toHaveLength(2)
  })

  it('requires explicit BT file choice and sends fileId with the original opaque episodeId', async () => {
    const request = vi
      .spyOn(api, 'get')
      .mockImplementation(async (url, config) => {
        if (String(url).endsWith('/search')) return searchReady('bt-1')
        if (String(url).includes('/subjects/')) return lines('bt-1')
        if (!config?.params.fileId) {
          return {
            data: {
              sourceId: 'bt-1',
              state: 'selection_required',
              format: 'torrent',
              files: [
                { id: 'bound-small', name: 'Episode SP.mp4', size: 1024 },
                { id: 'bound-large', name: 'Entire season.mkv', size: 999999 },
              ],
            },
          }
        }
        return {
          data: {
            sourceId: 'bt-1',
            state: 'ready',
            streamUrl: '/admitted/selected-file',
            format: 'mp4',
          },
        }
      })
    mount([btSources[1]])
    fireEvent.click(screen.getByRole('tab', { name: /BitTorrent sources/ }))
    fireEvent.click(
      await screen.findByRole('button', { name: btSources[1].name })
    )
    fireEvent.click(
      (await screen.findAllByRole('button', { name: 'Same label' }))[1]
    )
    fireEvent.click(screen.getByRole('button', { name: 'Special episode' }))
    const files = await screen.findByRole('region', {
      name: 'Choose a torrent file',
    })
    expect(play).not.toHaveBeenCalled()
    fireEvent.click(
      within(files).getByRole('button', { name: /Episode SP.mp4/ })
    )
    await waitFor(() =>
      expect(play).toHaveBeenCalledWith(
        expect.objectContaining({ streamUrl: '/admitted/selected-file' })
      )
    )
    expect(request).toHaveBeenLastCalledWith(
      '/api/bangumi/sources/bt-1/resolve',
      expect.objectContaining({
        params: { episodeId: 'episode/b', fileId: 'bound-small' },
      })
    )
  })

  it('does not let a late resolution replace the selected line after selection changes', async () => {
    let resolveEpisode: ((value: { data: object }) => void) | undefined
    let resolvingSignal: AbortSignal | undefined
    vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
      if (String(url).endsWith('/search')) return searchReady('web-0')
      if (String(url).includes('/subjects/')) return lines()
      resolvingSignal = config?.signal as AbortSignal
      return new Promise((resolve) => {
        resolveEpisode = resolve
      })
    })
    mount([webSources[0]])
    await openFirst()
    fireEvent.click(screen.getAllByRole('button', { name: 'Same label' })[0])
    fireEvent.click(screen.getByRole('button', { name: 'Decimal episode' }))
    await screen.findByText('Resolving episode…')
    fireEvent.click(screen.getAllByRole('button', { name: 'Same label' })[1])
    await waitFor(() => expect(resolvingSignal?.aborted).toBe(true))
    await act(async () =>
      resolveEpisode?.({
        data: { sourceId: 'web-0', state: 'ready', streamUrl: '/stale/video' },
      })
    )
    expect(play).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Special episode' })
    ).toBeVisible()
  })
})
