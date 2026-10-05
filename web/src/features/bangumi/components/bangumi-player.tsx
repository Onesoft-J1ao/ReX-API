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
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Label } from '@/components/ui/label'
import {
  Progress,
  ProgressIndicator,
  ProgressTrack,
} from '@/components/ui/progress'
import { Switch } from '@/components/ui/switch'

import { useTorrentPlayback } from '../hooks/use-torrent-playback'
import type { DanmakuComment } from '../lib/danmaku'
import {
  type PlaybackFailure,
  type PlaybackStatus,
  formatBytes,
  shouldShowOverlay,
} from '../lib/playback-state'
import { DanmakuOverlay } from './danmaku-overlay'

/**
 * Why playback stopped, in the viewer's terms.
 *
 * `noPeers` is the one worth reading: a release whose seeders are ordinary
 * BitTorrent clients has nobody a browser can reach, so the transfer never
 * starts and the visitor needs to know that is what happened rather than
 * assume the page is broken.
 *
 * `unknownError` is deliberately absent: the title already says playback
 * failed, and there is no more specific reason to give behind it.
 */
const FAILURE_DESCRIPTIONS: Partial<Record<PlaybackFailure, string>> = {
  resolutionTimedOut: 'The source did not respond in time.',
  networkError: 'Could not reach the source.',
  unsupportedMedia: 'This browser cannot play this file.',
  noMatchingFile: 'This release contains no playable video file.',
  noPeers:
    'No peers are sharing this release over WebRTC. It may only be seeding to regular BitTorrent clients.',
}

const DANMAKU_SWITCH_ID = 'bangumi-danmaku-switch'

/**
 * Plays one release, with its bullet comments drawn over the video.
 *
 * The video element is always mounted so the torrent has somewhere to render
 * as soon as a file is chosen; the overlay sits on top of it until playback
 * genuinely starts.
 */
export function BangumiPlayer(props: {
  torrentUrl: string | null
  episodeNumber?: number
  comments: DanmakuComment[]
}) {
  const { t } = useTranslation()
  const [danmakuEnabled, setDanmakuEnabled] = useState(true)
  const [video, setVideo] = useState<HTMLVideoElement | null>(null)

  const {
    status,
    failure,
    numPeers,
    downloadSpeed,
    progress,
    fileName,
    videoRef,
    retry,
  } = useTorrentPlayback({
    torrentUrl: props.torrentUrl,
    episodeNumber: props.episodeNumber,
  })

  // The element is needed twice: the hook renders the torrent into it, and the
  // danmaku layer reads its playhead.
  const attachVideo = useCallback(
    (node: HTMLVideoElement | null) => {
      videoRef(node)
      setVideo(node)
    },
    [videoRef]
  )

  const speed = formatBytes(downloadSpeed)

  return (
    <div className='space-y-3'>
      <div className='relative aspect-video w-full overflow-hidden rounded-lg bg-black'>
        <video ref={attachVideo} controls playsInline className='h-full w-full' />
        <DanmakuOverlay
          comments={props.comments}
          video={video}
          enabled={danmakuEnabled}
        />
        {shouldShowOverlay(status) && (
          <div className='absolute inset-0 flex items-center justify-center bg-black/70 p-4'>
            <PlaybackOverlay
              status={status}
              failure={failure}
              numPeers={numPeers}
              progress={progress}
              onRetry={retry}
            />
          </div>
        )}
      </div>

      <div className='flex flex-wrap items-center justify-between gap-3'>
        <p className='text-muted-foreground min-w-0 truncate text-xs'>
          {fileName ??
            (speed
              ? t('{{speed}}/s', { speed })
              : t('{{count}} peers', { count: numPeers }))}
        </p>
        <div className='flex items-center gap-2'>
          <Switch
            id={DANMAKU_SWITCH_ID}
            size='sm'
            checked={danmakuEnabled}
            onCheckedChange={setDanmakuEnabled}
          />
          <Label htmlFor={DANMAKU_SWITCH_ID} className='text-xs'>
            {t('Danmaku')}
          </Label>
        </div>
      </div>
    </div>
  )
}

/**
 * What covers the video while playback is not running.
 *
 * Buffering is split by whether any peer has been found at all: a transfer with
 * zero peers is not slow, it is stuck, and saying so is more useful than a
 * spinner that looks identical either way.
 */
function PlaybackOverlay(props: {
  status: PlaybackStatus
  failure: PlaybackFailure | null
  numPeers: number
  progress: number
  onRetry: () => void
}) {
  const { t } = useTranslation()

  if (props.status === 'failed') {
    const description = props.failure
      ? FAILURE_DESCRIPTIONS[props.failure]
      : undefined
    return (
      <ErrorState
        className='min-h-0'
        title={t('Playback failed')}
        description={description ? t(description) : undefined}
        onRetry={props.onRetry}
      />
    )
  }

  if (props.status === 'idle') {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('Pick an episode to start watching.')}
      </p>
    )
  }

  if (props.status === 'resolving') {
    return <LoadingState message={t('Fetching the source…')} />
  }

  const message =
    props.numPeers === 0 ? t('Looking for peers…') : t('Buffering…')

  return (
    <div className='w-full max-w-sm space-y-2'>
      <LoadingState message={message} />
      <Progress value={Math.round(props.progress * 100)}>
        <ProgressTrack>
          <ProgressIndicator />
        </ProgressTrack>
      </Progress>
      <p className='text-muted-foreground text-center text-xs'>
        {t('{{count}} peers', { count: props.numPeers })}
      </p>
    </div>
  )
}
