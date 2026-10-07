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
import type Hls from 'hls.js'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Kbd, KbdGroup } from '@/components/ui/kbd'
import { LoadingState } from '@/components/loading-state'

import { filterDanmaku, parseBlockedWords, type DanmakuComment } from '../lib/danmaku'
import {
  PLAYER_SPEEDS,
  readPreference,
  writePreference,
} from '../lib/player-preference'
import {
  isFinished,
  readProgress,
  resumePosition,
  writeProgress,
} from '../lib/playback-progress'
import { type PlaybackStatus, shouldShowOverlay } from '../lib/playback-state'
import { BangumiPlayerSettings } from './bangumi-player-settings'
import { DanmakuOverlay } from './danmaku-overlay'

const SPEED_KEY = 'bangumi:player:speed'
const BLOCKED_KEY = 'bangumi:player:blocked-words'

/** How often the position is written while playing, in milliseconds. */
const PROGRESS_SAVE_INTERVAL_MS = 5000
const SEEK_STEP_SECONDS = 5
const VOLUME_STEP = 0.1

function readSpeed(): number {
  const stored = Number(readPreference(SPEED_KEY))
  return (PLAYER_SPEEDS as readonly number[]).includes(stored) ? stored : 1
}

export function BangumiPlayer(props: {
  streamUrl: string | null
  format?: string
  title: string
  comments: DanmakuComment[]
  /** Identifies the episode for resume; must be stable across resolves. */
  progressKey: string
  /**
   * Called when the media itself fails. A resolve that succeeded only proves the
   * server could reach the stream; the browser can still refuse to decode it,
   * and the caller uses this to try another source.
   */
  onFailed?: () => void
}) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [video, setVideo] = useState<HTMLVideoElement | null>(null)
  const [status, setStatus] = useState<PlaybackStatus>('idle')
  const [attempt, setAttempt] = useState(0)
  const failedRef = useRef(props.onFailed)
  failedRef.current = props.onFailed
  const [speed, setSpeed] = useState(readSpeed)
  const [danmakuEnabled, setDanmakuEnabled] = useState(true)
  const [blockedWords, setBlockedWords] = useState(
    () => readPreference(BLOCKED_KEY) ?? ''
  )
  const visibleComments = useMemo(
    () => filterDanmaku(props.comments, parseBlockedWords(blockedWords)),
    [props.comments, blockedWords]
  )

  useEffect(() => {
    const streamUrl = props.streamUrl
    if (!video || !streamUrl) {
      setStatus('idle')
      return
    }
    let cancelled = false
    let hls: Hls | null = null

    const onPlaying = () => setStatus('playing')
    const onPause = () => setStatus(video.ended ? 'ended' : 'paused')
    const onEnded = () => setStatus('ended')
    const onWaiting = () => setStatus('loading')
    const onReady = () => setStatus(video.paused ? 'paused' : 'playing')
    const onError = () => setStatus('failed')

    video.addEventListener('playing', onPlaying)
    video.addEventListener('pause', onPause)
    video.addEventListener('ended', onEnded)
    video.addEventListener('waiting', onWaiting)
    video.addEventListener('loadstart', onWaiting)
    video.addEventListener('canplay', onReady)
    video.addEventListener('error', onError)
    setStatus('loading')
    const loadSource = async () => {
      if (props.format !== 'hls' || video.canPlayType('application/vnd.apple.mpegurl')) {
        video.src = streamUrl
        video.load()
        return
      }
      try {
        const { default: HlsConstructor } = await import('hls.js')
        if (cancelled) return
        if (!HlsConstructor.isSupported()) {
          onError()
          return
        }
        hls = new HlsConstructor({ maxBufferLength: 20, maxMaxBufferLength: 45, backBufferLength: 15 })
        hls.on(HlsConstructor.Events.ERROR, (_event, error) => {
          if (!cancelled && error.fatal) onError()
        })
        hls.attachMedia(video)
        hls.loadSource(streamUrl)
      } catch {
        if (!cancelled) onError()
      }
    }
    void loadSource()

    return () => {
      cancelled = true
      video.removeEventListener('playing', onPlaying)
      video.removeEventListener('pause', onPause)
      video.removeEventListener('ended', onEnded)
      video.removeEventListener('waiting', onWaiting)
      video.removeEventListener('loadstart', onWaiting)
      video.removeEventListener('canplay', onReady)
      video.removeEventListener('error', onError)
      hls?.destroy()
      // Removing the source and loading again aborts outstanding media requests.
      video.pause()
      video.removeAttribute('src')
      video.load()
    }
  }, [video, props.streamUrl, props.format, attempt])

  // Resume where this episode was left, and keep the position current. The
  // record outlives the media URL, which the server regenerates on every
  // resolve, so it is keyed by the episode rather than the stream.
  useEffect(() => {
    const streamUrl = props.streamUrl
    if (!video || !streamUrl) return
    const key = props.progressKey
    let lastSaved = 0

    const save = (finished: boolean) => {
      const duration = video.duration
      if (!Number.isFinite(duration) || duration <= 0) return
      writeProgress(key, {
        position: video.currentTime,
        duration,
        finished: finished || isFinished(video.currentTime, duration),
        updatedAt: Date.now(),
      })
    }
    const onLoadedMetadata = () => {
      const position = resumePosition(readProgress(key), video.duration)
      if (position > 0) video.currentTime = position
    }
    const onTimeUpdate = () => {
      if (Date.now() - lastSaved < PROGRESS_SAVE_INTERVAL_MS) return
      lastSaved = Date.now()
      save(false)
    }
    const onPause = () => save(false)
    const onEnded = () => save(true)

    video.addEventListener('loadedmetadata', onLoadedMetadata)
    video.addEventListener('timeupdate', onTimeUpdate)
    video.addEventListener('pause', onPause)
    video.addEventListener('ended', onEnded)
    return () => {
      video.removeEventListener('loadedmetadata', onLoadedMetadata)
      video.removeEventListener('timeupdate', onTimeUpdate)
      video.removeEventListener('pause', onPause)
      video.removeEventListener('ended', onEnded)
      // A switch to another episode, source or page is the last chance to keep
      // this position.
      save(false)
    }
  }, [video, props.streamUrl, props.progressKey])

  useEffect(() => {
    if (video) video.playbackRate = speed
  }, [video, speed])

  // Report the failure once per transition, so the caller can move on rather
  // than re-trying a stream that will not decode.
  useEffect(() => {
    if (status === 'failed') failedRef.current?.()
  }, [status])

  // Shortcuts follow the page rather than the element: the native controls own
  // the keys while they have focus, so a handler on the container would fight
  // them. Typing in a field is left alone, and every key handled here is
  // consumed so the page does not also scroll.
  useEffect(() => {
    const streamUrl = props.streamUrl
    if (!video || !streamUrl) return
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      if (
        event.metaKey ||
        event.ctrlKey ||
        event.altKey ||
        (target &&
          (target.isContentEditable ||
            ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)))
      ) {
        return
      }
      const seek = (delta: number) => {
        const duration = video.duration
        const next = video.currentTime + delta
        video.currentTime = Number.isFinite(duration)
          ? Math.min(Math.max(next, 0), duration)
          : Math.max(next, 0)
      }
      const cycleSpeed = (step: number) => {
        const current = (PLAYER_SPEEDS as readonly number[]).indexOf(speed)
        const index = current === -1 ? 0 : current + step
        const clamped = Math.min(Math.max(index, 0), PLAYER_SPEEDS.length - 1)
        setSpeed(PLAYER_SPEEDS[clamped])
      }
      switch (event.key) {
        case ' ':
        case 'k':
          if (video.paused) void video.play()
          else video.pause()
          break
        case 'ArrowLeft':
          seek(-SEEK_STEP_SECONDS)
          break
        case 'ArrowRight':
          seek(SEEK_STEP_SECONDS)
          break
        case 'ArrowUp':
          video.volume = Math.min(video.volume + VOLUME_STEP, 1)
          break
        case 'ArrowDown':
          video.volume = Math.max(video.volume - VOLUME_STEP, 0)
          break
        case 'm':
          video.muted = !video.muted
          break
        case 'f':
          if (document.fullscreenElement) void document.exitFullscreen()
          else void containerRef.current?.requestFullscreen()
          break
        case '[':
          cycleSpeed(-1)
          break
        case ']':
          cycleSpeed(1)
          break
        default:
          return
      }
      event.preventDefault()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [video, props.streamUrl, speed])

  const changeSpeed = (next: number) => {
    setSpeed(next)
    writePreference(SPEED_KEY, String(next))
  }
  const changeBlockedWords = (value: string) => {
    setBlockedWords(value)
    writePreference(BLOCKED_KEY, value)
  }

  return (
    <div className='space-y-3'>
      <div
        ref={containerRef}
        className='relative aspect-video w-full overflow-hidden rounded-xl bg-black'
      >
        <video
          ref={setVideo}
          aria-label={props.title}
          controls
          playsInline
          preload='metadata'
          className='h-full w-full'
        />
        <DanmakuOverlay
          comments={visibleComments}
          video={video}
          enabled={danmakuEnabled}
        />
        {shouldShowOverlay(status) && status !== 'failed' && (
          <div
            role='status'
            aria-live='polite'
            className='pointer-events-none absolute inset-x-0 top-0 bottom-12 flex items-center justify-center p-3'
          >
            <div className='bg-background text-foreground max-w-full rounded-xl p-3 text-center text-sm shadow-sm'>
              {status === 'idle' ? (
                t('Pick an episode to start watching.')
              ) : (
                <LoadingState inline message={t('Buffering…')} />
              )}
            </div>
          </div>
        )}
      </div>

      {status === 'failed' && (
        <ErrorState
          className='min-h-0 p-3'
          title={t('Playback failed')}
          description={t('This source could not be played. Try another one.')}
          onRetry={() => setAttempt((value) => value + 1)}
        />
      )}

      <BangumiPlayerSettings
        speed={speed}
        onSpeedChange={changeSpeed}
        danmakuEnabled={danmakuEnabled}
        onDanmakuEnabledChange={setDanmakuEnabled}
        blockedWords={blockedWords}
        onBlockedWordsChange={changeBlockedWords}
      />

      {props.streamUrl && (
        <p className='text-muted-foreground hidden flex-wrap items-center gap-x-3 gap-y-1 text-xs sm:flex'>
          <KbdGroup>
            <Kbd>Space</Kbd>
            <span>{t('Play or pause')}</span>
          </KbdGroup>
          <KbdGroup>
            <Kbd>←</Kbd>
            <Kbd>→</Kbd>
            <span>{t('Seek 5 seconds')}</span>
          </KbdGroup>
          <KbdGroup>
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd>
            <span>{t('Volume')}</span>
          </KbdGroup>
          <KbdGroup>
            <Kbd>M</Kbd>
            <span>{t('Mute')}</span>
          </KbdGroup>
          <KbdGroup>
            <Kbd>F</Kbd>
            <span>{t('Full screen')}</span>
          </KbdGroup>
          <KbdGroup>
            <Kbd>[</Kbd>
            <Kbd>]</Kbd>
            <span>{t('Speed')}</span>
          </KbdGroup>
        </p>
      )}
    </div>
  )
}
