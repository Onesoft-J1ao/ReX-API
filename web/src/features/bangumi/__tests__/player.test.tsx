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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { BangumiPlayer } from '../components/bangumi-player'

beforeEach(() => {
  window.localStorage.clear()
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue()
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
})

/** The player renders exactly one video; the label is how the tests find it. */
function getVideo(label: string): HTMLVideoElement {
  return screen.getByLabelText(label) as HTMLVideoElement
}

/**
 * jsdom implements neither the media clock nor the playback rate, so the two
 * properties the player reads and writes are backed by plain fields here.
 */
function stubMediaClock(video: HTMLVideoElement, duration: number) {  const state = { currentTime: 0 }
  Object.defineProperty(video, 'duration', { configurable: true, value: duration })
  Object.defineProperty(video, 'currentTime', {
    configurable: true,
    get: () => state.currentTime,
    set: (value: number) => {
      state.currentTime = value
    },
  })
  return state
}

describe('anime player', () => {
  it('uses the browser native HLS player when it is available', () => {
    vi.spyOn(HTMLMediaElement.prototype, 'canPlayType').mockReturnValue('probably')
    render(<BangumiPlayer streamUrl='/playlist.m3u8' format='hls' title='HLS episode' comments={[]} progressKey='episode' />)
    expect(screen.getByLabelText('HLS episode')).toHaveAttribute('src', '/playlist.m3u8')
  })

  it('reports an unsupported HLS environment instead of buffering forever', async () => {
    vi.spyOn(HTMLMediaElement.prototype, 'canPlayType').mockReturnValue('')
    render(<BangumiPlayer streamUrl='/playlist.m3u8' format='hls' title='HLS episode' comments={[]} progressKey='episode' />)
    expect(await screen.findByText('Playback failed')).toBeVisible()
    expect(screen.queryByText('Buffering…')).not.toBeInTheDocument()
  })

  it('leaves controls reachable while loading and clears the overlay when playback is ready', () => {
    render(
      <BangumiPlayer streamUrl='/video/1' title='Episode one' comments={[]} progressKey='episode' />
    )
    const video = getVideo('Episode one')
    expect(video).toHaveAttribute('controls')
    expect(screen.getByRole('status')).toHaveClass('pointer-events-none')
    fireEvent.canPlay(video)
    expect(screen.queryByText('Buffering…')).not.toBeInTheDocument()
    fireEvent.waiting(video)
    expect(screen.getByText('Buffering…')).toBeVisible()
    fireEvent.playing(video)
    expect(screen.queryByText('Buffering…')).not.toBeInTheDocument()
  })

  it('shows a retry action on media error and reloads the same source', () => {
    render(
      <BangumiPlayer streamUrl='/video/1' title='Episode one' comments={[]} progressKey='episode' />
    )
    const video = getVideo('Episode one')
    fireEvent.error(video)
    expect(screen.getByText('Playback failed')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(screen.queryByText('Playback failed')).not.toBeInTheDocument()
    expect(video).toHaveAttribute('src', '/video/1')
    fireEvent.canPlay(video)
    expect(screen.queryByText('Buffering…')).not.toBeInTheDocument()
  })

  it('detaches the media URL on source removal and unmount', () => {
    const view = render(
      <BangumiPlayer streamUrl='/video/1' title='Episode one' comments={[]} progressKey='episode' />
    )
    const video = getVideo('Episode one')
    view.rerender(
      <BangumiPlayer streamUrl={null} title='Episode one' comments={[]} progressKey='episode' />
    )
    expect(video).not.toHaveAttribute('src')
    expect(screen.getByText('Pick an episode to start watching.')).toBeVisible()
    view.rerender(
      <BangumiPlayer streamUrl='/video/2' title='Episode two' comments={[]} progressKey='episode' />
    )
    expect(video).toHaveAttribute('src', '/video/2')
    view.unmount()
    expect(video).not.toHaveAttribute('src')
  })

  it('resumes an unfinished episode and starts a finished one over', () => {
    window.localStorage.setItem(
      'bangumi:progress:42:3',
      JSON.stringify({ position: 300, duration: 1200, finished: false, updatedAt: 1 })
    )
    const view = render(
      <BangumiPlayer streamUrl='/video/1' title='Episode three' comments={[]} progressKey='42:3' />
    )
    const video = getVideo('Episode three')
    const resumed = stubMediaClock(video, 1200)
    fireEvent.loadedMetadata(video)
    expect(resumed.currentTime).toBe(300)

    // A finished episode is remembered but not resumed.
    view.unmount()
    window.localStorage.setItem(
      'bangumi:progress:42:4',
      JSON.stringify({ position: 1190, duration: 1200, finished: true, updatedAt: 2 })
    )
    render(
      <BangumiPlayer streamUrl='/video/1' title='Episode four' comments={[]} progressKey='42:4' />
    )
    const finished = getVideo('Episode four')
    const restarted = stubMediaClock(finished, 1200)
    fireEvent.loadedMetadata(finished)
    expect(restarted.currentTime).toBe(0)
  })

  it('remembers where playback was left when the episode is switched away', () => {
    const view = render(
      <BangumiPlayer streamUrl='/video/1' title='Episode one' comments={[]} progressKey='42:1' />
    )
    const video = getVideo('Episode one')
    const clock = stubMediaClock(video, 600)
    clock.currentTime = 240
    view.unmount()
    const stored = JSON.parse(
      window.localStorage.getItem('bangumi:progress:42:1') ?? 'null'
    )
    expect(stored).toMatchObject({ position: 240, duration: 600, finished: false })
  })

  it('applies the chosen speed and keeps it for the next episode', () => {
    const view = render(
      <BangumiPlayer streamUrl='/video/1' title='Episode one' comments={[]} progressKey='episode' />
    )
    const video = getVideo('Episode one')
    fireEvent.change(screen.getByLabelText('Playback speed'), { target: { value: '1.5' } })
    expect(video.playbackRate).toBe(1.5)
    expect(window.localStorage.getItem('bangumi:player:speed')).toBe('1.5')

    view.unmount()
    render(
      <BangumiPlayer streamUrl='/video/2' title='Episode two' comments={[]} progressKey='episode-2' />
    )
    expect(getVideo('Episode two').playbackRate).toBe(1.5)
  })

  it('handles the playback shortcuts from the page', () => {
    render(
      <BangumiPlayer streamUrl='/video/1' title='Episode one' comments={[]} progressKey='episode' />
    )
    const video = getVideo('Episode one')
    const clock = stubMediaClock(video, 100)
    clock.currentTime = 10
    const play = vi.mocked(HTMLMediaElement.prototype.play)

    fireEvent.keyDown(window, { key: ' ' })
    expect(play).toHaveBeenCalled()
    fireEvent.keyDown(window, { key: 'ArrowRight' })
    expect(clock.currentTime).toBe(15)
    fireEvent.keyDown(window, { key: 'ArrowLeft' })
    expect(clock.currentTime).toBe(10)
    // A key that is not ours is left to the page.
    fireEvent.keyDown(window, { key: 'x' })
    expect(clock.currentTime).toBe(10)
  })

  it('does not treat typing in a field as a shortcut', () => {
    render(
      <BangumiPlayer streamUrl='/video/1' title='Episode one' comments={[]} progressKey='episode' />
    )
    const video = getVideo('Episode one')
    const clock = stubMediaClock(video, 100)
    clock.currentTime = 10
    fireEvent.keyDown(screen.getByLabelText('Blocked words'), { key: 'ArrowRight' })
    expect(clock.currentTime).toBe(10)
  })

  it('keeps the danmaku block list in the viewer storage', () => {
    render(
      <BangumiPlayer streamUrl='/video/1' title='Episode one' comments={[]} progressKey='episode' />
    )
    fireEvent.change(screen.getByLabelText('Blocked words'), {
      target: { value: '剧透, 广告' },
    })
    expect(window.localStorage.getItem('bangumi:player:blocked-words')).toBe(
      '剧透, 广告'
    )
  })
})
