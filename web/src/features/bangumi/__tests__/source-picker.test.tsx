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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { BangumiSourcePicker } from '../components/bangumi-source-picker'
import type { BangumiRelease } from '../types'

const releases: BangumiRelease[] = [1, 2, 0].map((number) => ({
  topicId: String(number),
  title: `Release ${number}`,
  alliance: number === 2 ? 'Other group' : 'Group',
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

describe('source groups', () => {
  it('shows the current episode and batches by default, not another episode', () => {
    const selected = vi.fn()
    render(
      <BangumiSourcePicker
        releases={releases}
        selectedId='1'
        episodeNumber={1}
        onSelect={selected}
      />
    )
    expect(screen.getByRole('button', { name: 'Release 1' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Release 0' })).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Release 2' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Other group/ })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Release 1' }))
    expect(selected).toHaveBeenCalledWith(releases[0])
  })

  it('makes other episode groups available when the viewer explicitly asks for all episodes', () => {
    render(
      <BangumiSourcePicker
        releases={releases}
        selectedId='1'
        episodeNumber={1}
        onSelect={vi.fn()}
      />
    )
    fireEvent.click(screen.getByRole('switch', { name: 'Show all episodes' }))
    const trigger = screen.getByRole('button', { name: /Other group/ })
    fireEvent.click(trigger)
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('button', { name: 'Release 2' })).toBeVisible()
  })

  it('shows the empty state for an unmatched filter and recovers when it is cleared', () => {
    render(
      <BangumiSourcePicker
        releases={releases}
        selectedId='1'
        episodeNumber={1}
        onSelect={vi.fn()}
      />
    )
    const filter = screen.getByRole('textbox', { name: 'Filter releases' })
    fireEvent.change(filter, { target: { value: 'missing group' } })
    expect(
      screen.queryByRole('button', { name: 'Release 1' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByText('No source matched this episode. Pick one below.')
    ).toBeVisible()
    fireEvent.change(filter, { target: { value: '' } })
    expect(screen.getByRole('button', { name: 'Release 1' })).toBeVisible()
  })
})
