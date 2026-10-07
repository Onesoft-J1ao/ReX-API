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

import { BangumiSubjectPage } from '../bangumi-subject-dialog'
import type { BangumiRelease } from '../types'

const title = 'A long anime title'
const releases: BangumiRelease[] = [1, 2].map((number) => ({
  topicId: String(number),
  title: `Release ${number}`,
  alliance: 'Group',
  description: '',
  downloadLink: `https://mikanani.me/Download/${number}.torrent`,
  episodeNumber: number,
  size: 1000,
  publishedAt: '',
  publishedTime: 0,
  resolution: '1080P',
  subtitleLanguages: ['CHS'],
  matchKind: 'EXACT',
  playable: true,
  videoCodec: 'H264',
  container: 'MP4',
}))
const episodes = releases.map((release, index) => ({
  episodeId: 100 + index,
  episodeTitle: `Episode ${index + 1}`,
  episodeNumber: String(index + 1),
  release,
}))
let client: QueryClient

beforeEach(() => {
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['bangumi', 'sources'], {
    sources: [
      {
        id: 'mikan-mikanime-tv',
        name: 'Mikan',
        kind: 'bt',
        icon: '',
        tier: 0,
        enabled: true,
      },
    ],
    legacyMikanSourceId: 'mikan-mikanime-tv',
  })
  client.setQueryData(
    ['bangumi', 'calendar'],
    [
      {
        weekday: { id: 1, en: 'Monday' },
        items: [{ id: 42, name: title, type: 2 }],
      },
    ]
  )
  client.setQueryData(['bangumi', 'subject', 42], {
    id: 42,
    name: title,
    type: 2,
  })
  client.setQueryData(['bangumi', 'episodes', 42], {
    animeId: 42,
    animeTitle: title,
    episodes,
  })
  client.setQueryData(['bangumi', 'releases', 42, title], {
    subjectId: 42,
    matchKind: 'EXACT',
    releases,
    episodes,
  })
  for (const episode of episodes) {
    client.setQueryData(['bangumi', 'danmaku', episode.episodeId], {
      count: 0,
      comments: [],
    })
  }
})

afterEach(() => {
  client.clear()
})

// The detail and player are their own pages now, so this drives them by
// mounting the detail page and pressing Watch rather than clicking a calendar
// card that would navigate. What the flow covers is unchanged: pick a source,
// play an episode, switch episodes, and release the video on the way out.
async function openWatch(): Promise<void> {
  render(
    <QueryClientProvider client={client}>
      <BangumiSubjectPage subjectId={42} onBack={vi.fn()} />
    </QueryClientProvider>
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Watch' }))
  fireEvent.click(
    await screen.findByRole('tab', { name: /BitTorrent sources/ })
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Mikan' }))
}

describe('anime watching', () => {
  it('keeps the subject when Watch opens, selects another episode and releases the video on close', async () => {
    await openWatch()
    const first = await screen.findByLabelText('Episode 1', {
      selector: 'video',
    })
    expect(first).toHaveAttribute(
      'src',
      expect.stringContaining('1.torrent?episode=1')
    )
    fireEvent.click(screen.getByRole('button', { name: 'Episode 2' }))
    const second = await screen.findByLabelText('Episode 2', {
      selector: 'video',
    })
    await waitFor(() =>
      expect(second).toHaveAttribute(
        'src',
        expect.stringContaining('2.torrent?episode=2')
      )
    )
    expect(screen.getByRole('button', { name: 'Episode 2' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    // Leaving the player unmounts it, which is the release path now that the
    // player is a page instead of a dialog with a close button.
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    await waitFor(() =>
      expect(
        screen.queryByLabelText('Episode 2', { selector: 'video' })
      ).not.toBeInTheDocument()
    )
    expect(second).not.toHaveAttribute('src')
  })

  it('shows a retryable source error without misreporting an empty source list', async () => {
    client.removeQueries({ queryKey: ['bangumi', 'releases'] })
    const request = vi.spyOn(api, 'get')
    request.mockRejectedValueOnce(new Error('offline'))
    await openWatch()
    expect(await screen.findByText('Could not load this source')).toBeVisible()
    expect(
      screen.queryByText('No sources found for this anime.')
    ).not.toBeInTheDocument()
    request.mockResolvedValueOnce({
      data: { subjectId: 42, matchKind: 'EXACT', releases, episodes },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    const video = await screen.findByLabelText('Episode 1', {
      selector: 'video',
    })
    await waitFor(() =>
      expect(video).toHaveAttribute('src', expect.stringContaining('1.torrent'))
    )
  })
})
