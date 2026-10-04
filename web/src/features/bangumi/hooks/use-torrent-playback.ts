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
import { useCallback, useEffect, useRef, useState } from 'react'
import type { Torrent, TorrentFile } from 'webtorrent'

import {
  type PlaybackFailure,
  type PlaybackStatus,
  isStalled,
} from '../lib/playback-state'
import { selectVideoFile } from '../lib/torrent-file'

/**
 * The browser build's constructor and instance.
 *
 * Referenced as types only: the library is loaded on demand, so nothing may
 * pull it into the page's initial bundle. The package's own ESM entry imports
 * Node builtins a browser build cannot resolve, which is why the pre-bundled
 * `dist/webtorrent.min.js` is what gets loaded.
 */
type WebTorrentConstructor = typeof import('webtorrent/dist/webtorrent.min.js').default
type WebTorrentClient = InstanceType<WebTorrentConstructor>

/**
 * Public WebSocket trackers, announced on every torrent.
 *
 * A page can only reach a swarm over WebRTC, and WebRTC peers are discovered
 * through a WebSocket tracker. The UDP and HTTP trackers Mikan publishes are
 * unreachable from a browser, so without these a torrent would never be
 * announced to anything the page can actually talk to.
 */
const WEBRTC_TRACKERS = [
  'wss://tracker.openwebtorrent.com',
  'wss://tracker.btorrent.xyz',
  'wss://tracker.webtorrent.dev',
]

/** How often the transfer's speed and progress are sampled. */
const SAMPLE_INTERVAL_MS = 1_000

export type TorrentPlayback = {
  status: PlaybackStatus
  failure: PlaybackFailure | null
  numPeers: number
  /** Bytes per second. */
  downloadSpeed: number
  /** 0..1 */
  progress: number
  fileName: string | null
  /** Attach to the `<video>` the torrent is rendered into. */
  videoRef: (node: HTMLVideoElement | null) => void
  retry: () => void
}

/**
 * Streams a torrent into a `<video>` element and reports what the transfer is
 * doing.
 *
 * No video data passes through this instance: the browser joins the swarm
 * itself. That is also why the failure states carry weight. A release whose
 * seeders speak only BitTorrent's TCP protocol has no WebRTC peer to connect
 * to, so the transfer sits at zero indefinitely, and the viewer has to be told
 * that plainly rather than shown a spinner that never resolves.
 */
export function useTorrentPlayback(options: {
  torrentUrl: string | null
  episodeNumber?: number
}): TorrentPlayback {
  const [video, setVideo] = useState<HTMLVideoElement | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [status, setStatus] = useState<PlaybackStatus>('idle')
  const [failure, setFailure] = useState<PlaybackFailure | null>(null)
  const [numPeers, setNumPeers] = useState(0)
  const [downloadSpeed, setDownloadSpeed] = useState(0)
  const [progress, setProgress] = useState(0)
  const [fileName, setFileName] = useState<string | null>(null)
  const [file, setFile] = useState<TorrentFile | null>(null)

  // The sampling interval runs outside React's render cycle, so it reads the
  // chosen file through a ref rather than through the state it would otherwise
  // capture stale.
  const chosenRef = useRef<TorrentFile | null>(null)

  const videoRef = useCallback((node: HTMLVideoElement | null) => {
    setVideo(node)
  }, [])

  const retry = useCallback(() => {
    setAttempt((value) => value + 1)
  }, [])

  const { torrentUrl, episodeNumber } = options

  useEffect(() => {
    chosenRef.current = null
    setFileName(null)
    setFile(null)
    setNumPeers(0)
    setDownloadSpeed(0)
    setProgress(0)
    setFailure(null)

    if (!torrentUrl) {
      setStatus('idle')
      return
    }

    let cancelled = false
    let zeroSpeedSince: number | null = null
    let torrent: Torrent | null = null
    let client: WebTorrentClient | null = null
    let sampling = 0

    const fail = (reason: PlaybackFailure) => {
      if (cancelled) return
      setFailure(reason)
      setStatus('failed')
    }

    setStatus('resolving')

    const start = async () => {
      // Fetched on demand: the library is ~218 kB and is only needed once the
      // viewer actually plays something, so a visitor who only browses the
      // list never downloads it.
      let TorrentClient: WebTorrentConstructor
      try {
        ;({ default: TorrentClient } = await import(
          'webtorrent/dist/webtorrent.min.js'
        ))
      } catch {
        fail('networkError')
        return
      }
      if (cancelled) return

      client = new TorrentClient()
      try {
        torrent = client.add(
          torrentUrl,
          { announce: WEBRTC_TRACKERS },
          (ready) => {
            if (cancelled) return
            const chosen = selectVideoFile(ready.files, episodeNumber)
            if (!chosen) {
              fail('noMatchingFile')
              return
            }
            chosenRef.current = chosen
            setFileName(chosen.name)
            setFile(chosen)
            setStatus('buffering')
          }
        )
      } catch {
        fail('networkError')
        return
      }

      torrent.on('error', () => fail('networkError'))

      // Sampled rather than event-driven: WebTorrent exposes speed and progress
      // as live fields, not as events.
      sampling = window.setInterval(() => {
        if (cancelled || !torrent) return
        setNumPeers(torrent.numPeers)
        setDownloadSpeed(torrent.downloadSpeed)
        setProgress(torrent.progress)

        // A stall is only meaningful once a file has been chosen; before that
        // the client is still fetching the metadata and a zero speed is
        // expected.
        if (!chosenRef.current) return
        if (torrent.downloadSpeed === 0) {
          zeroSpeedSince ??= Date.now()
          if (isStalled(0, Date.now() - zeroSpeedSince)) fail('noPeers')
        } else {
          zeroSpeedSince = null
        }
      }, SAMPLE_INTERVAL_MS)
    }

    void start()

    return () => {
      cancelled = true
      window.clearInterval(sampling)
      // Releases the WebRTC peers and the in-memory store; without it every
      // episode the viewer opens leaks a client.
      client?.destroy()
    }
  }, [torrentUrl, episodeNumber, attempt])

  useEffect(() => {
    if (!file || !video) return

    file.renderTo(video, { autoplay: true })

    const onPlaying = () => setStatus('playing')
    const onPause = () => setStatus('paused')
    const onEnded = () => setStatus('ended')
    const onWaiting = () => setStatus('buffering')
    const onError = () => {
      setFailure('unsupportedMedia')
      setStatus('failed')
    }

    video.addEventListener('playing', onPlaying)
    video.addEventListener('pause', onPause)
    video.addEventListener('ended', onEnded)
    video.addEventListener('waiting', onWaiting)
    video.addEventListener('error', onError)
    return () => {
      video.removeEventListener('playing', onPlaying)
      video.removeEventListener('pause', onPause)
      video.removeEventListener('ended', onEnded)
      video.removeEventListener('waiting', onWaiting)
      video.removeEventListener('error', onError)
    }
  }, [file, video])

  return {
    status,
    failure,
    numPeers,
    downloadSpeed,
    progress,
    fileName,
    videoRef,
    retry,
  }
}
