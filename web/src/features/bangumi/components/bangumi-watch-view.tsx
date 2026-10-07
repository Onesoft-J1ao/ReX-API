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
import { ArrowLeft } from 'lucide-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'

import {
  bangumiStreamUrl,
  getBangumiDanmaku,
  getBangumiEpisodes,
  getBangumiReleases,
} from '../api'
import { useAutoSourceSelection } from '../hooks/use-auto-source-selection'
import { parseDanmaku } from '../lib/danmaku'
import type { BangumiMediaSelection, BangumiSourceState } from '../types'
import {
  BangumiMediaSources,
  type NativeMikanRequest,
} from './bangumi-media-sources'
import { BangumiPlayer } from './bangumi-player'
import { BangumiSourcePicker } from './bangumi-source-picker'

type WatchProps = {
  subjectId: number
  title: string
  onBack: () => void
}

export function BangumiWatchView(props: WatchProps) {
  return <BangumiWatchSession key={`${props.subjectId}:${props.title}`} {...props} />
}

function BangumiWatchSession(props: WatchProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [episodeId, setEpisodeId] = useState<number | null>(null)
  const [releaseTopicId, setReleaseTopicId] = useState<string | null>(null)
  const [nativeSelected, setNativeSelected] = useState(false)
  const [loadNativeReleases, setLoadNativeReleases] =
    useState<NativeMikanRequest | null>(null)
  const nativeRequest = useCallback(
    (signal: AbortSignal) =>
      getBangumiReleases(props.subjectId, props.title, signal),
    [props.subjectId, props.title]
  )
  const [selection, setSelection] = useState<BangumiMediaSelection | null>(null)
  const [danmakuEpisodeId, setDanmakuEpisodeId] = useState<number | null>(null)
  // The automatic pick runs until the viewer chooses something themselves; from
  // then on their choice wins, and a slow probe must not overwrite it.
  const [autoSuppressed, setAutoSuppressed] = useState(false)
  const [pickerRequested, setPickerRequested] = useState(false)
  const [autoPickedSourceId, setAutoPickedSourceId] = useState<string | null>(
    null
  )
  const sourcesPanelRef = useRef<HTMLDivElement | null>(null)
  const episodesQuery = useQuery({
    queryKey: ['bangumi', 'episodes', props.subjectId],
    queryFn: ({ signal }) => getBangumiEpisodes(props.subjectId, signal),
    staleTime: 60 * 60 * 1000,
    retry: false,
    meta: { errorToast: false },
  })
  const releasesQuery = useQuery({
    queryKey: ['bangumi', 'releases', props.subjectId, props.title],
    queryFn: ({ signal }) => {
      if (!loadNativeReleases) throw new Error('No native source selected')
      return loadNativeReleases(signal)
    },
    enabled: nativeSelected,
    staleTime: 30 * 60 * 1000,
    retry: false,
    meta: { errorToast: false },
  })
  const subjectId = props.subjectId
  const subjectTitle = props.title
  useEffect(() => {
    if (!nativeSelected) {
      void queryClient.cancelQueries({
        queryKey: ['bangumi', 'releases', subjectId, subjectTitle],
        exact: true,
      })
    }
  }, [nativeSelected, queryClient, subjectId, subjectTitle])
  const episodes =
    episodesQuery.data?.episodes ?? releasesQuery.data?.episodes ?? []
  const releases = releasesQuery.data?.releases ?? []
  const activeEpisode =
    episodes.find((episode) => episode.episodeId === episodeId) ?? episodes[0]
  const matchedRelease = releasesQuery.data?.episodes.find(
    (episode) => episode.episodeId === activeEpisode?.episodeId
  )?.release
  const selectedRelease = releases.find(
    (release) => release.topicId === releaseTopicId
  )
  const activeRelease = selectedRelease ?? matchedRelease
  const playbackEpisode =
    selectedRelease && selectedRelease.episodeNumber > 0
      ? episodes.find(
          (episode) =>
            episode.episodeNumber === String(selectedRelease.episodeNumber)
        )
      : activeEpisode
  const episodeLabel = playbackEpisode?.episodeNumber ?? ''
  let episodeNumber: number | undefined
  if (/^\d+$/.test(episodeLabel)) {
    const number = Number(episodeLabel)
    if (Number.isSafeInteger(number) && number > 0) episodeNumber = number
  }
  // Explicit single-episode releases remain useful when dandanplay is down.
  // An unknown batch must never become a request for the largest torrent file.
  if (
    episodeNumber == null &&
    selectedRelease &&
    Number.isSafeInteger(selectedRelease.episodeNumber) &&
    selectedRelease.episodeNumber > 0
  ) {
    episodeNumber = selectedRelease.episodeNumber
  }
  const nativeStreamUrl =
    nativeSelected && activeRelease && episodeNumber != null
      ? bangumiStreamUrl(activeRelease.downloadLink, episodeNumber)
      : null
  const streamUrl = selection?.streamUrl ?? nativeStreamUrl
  // Resume is keyed by the episode rather than the stream: the server hands out
  // a new media URL on every resolve and a source's episode id changes with the
  // catalogue, while the episode number is what the viewer thinks in.
  let progressKey = `${subjectId}:${props.title}`
  if (episodeNumber != null) progressKey = `${subjectId}:${episodeNumber}`
  else if (selection) progressKey = `${subjectId}:${selection.episode.id}`
  else if (playbackEpisode) progressKey = `${subjectId}:${playbackEpisode.episodeId}`
  const title = selection
    ? `${selection.subjectName} · ${selection.episode.name}`
    : (playbackEpisode?.episodeTitle ?? selectedRelease?.title ?? props.title)
  const knownDanmakuEpisode = nativeSelected
    ? playbackEpisode
    : episodes.find((episode) => episode.episodeId === danmakuEpisodeId)
  const activeDanmakuId = streamUrl ? knownDanmakuEpisode?.episodeId : undefined
  const danmakuQuery = useQuery({
    queryKey: ['bangumi', 'danmaku', activeDanmakuId],
    queryFn: ({ signal }) => {
      if (activeDanmakuId == null) {
        throw new Error('No known danmaku episode selected')
      }
      return getBangumiDanmaku(activeDanmakuId, signal)
    },
    enabled: activeDanmakuId != null,
    staleTime: 10 * 60 * 1000,
    retry: false,
    meta: { errorToast: false },
  })
  const comments = useMemo(
    () => parseDanmaku(danmakuQuery.data?.comments ?? []),
    [danmakuQuery.data]
  )
  const clearSelection = useCallback(() => {
    setSelection(null)
    setNativeSelected(false)
    setReleaseTopicId(null)
    setDanmakuEpisodeId(null)
  }, [])
  const selectNative = useCallback((loadReleases: NativeMikanRequest) => {
    setLoadNativeReleases(() => loadReleases)
    setNativeSelected(true)
  }, [])
  const playSource = useCallback((next: BangumiMediaSelection) => {
    setNativeSelected(false)
    setSelection(next)
    setDanmakuEpisodeId(null)
  }, [])
  // A manual choice both stops the automatic pick and takes the viewer to the
  // sources, so the page never sits on an error they cannot act on.
  const playManualSource = useCallback(
    (next: BangumiMediaSelection) => {
      setAutoSuppressed(true)
      setAutoPickedSourceId(null)
      playSource(next)
    },
    [playSource]
  )
  const changeManualSelection = useCallback(() => {
    setAutoSuppressed(true)
    clearSelection()
  }, [clearSelection])
  const selectManualNative = useCallback(
    (loadReleases: NativeMikanRequest) => {
      setAutoSuppressed(true)
      selectNative(loadReleases)
    },
    [selectNative]
  )
  const openSourcePicker = useCallback(() => {
    setAutoSuppressed(true)
    setPickerRequested(true)
    sourcesPanelRef.current?.scrollIntoView({
      behavior: 'smooth',
      block: 'start',
    })
  }, [])
  const applyAutoSelection = useCallback(
    (next: BangumiMediaSelection) => {
      setAutoPickedSourceId(next.sourceId)
      playSource(next)
    },
    [playSource]
  )
  const auto = useAutoSourceSelection({
    title: props.title,
    enabled: !autoSuppressed && selection == null && !nativeSelected,
    onSelected: applyAutoSelection,
  })
  const handlePlaybackFailed = useCallback(() => {
    // A resolve that succeeded only proves the server could reach the stream;
    // the browser can still refuse to decode it. When the automatic pick is the
    // one that failed, drop it and try the next source rather than leaving a
    // dead player on screen.
    if (!selection || selection.sourceId !== autoPickedSourceId) return
    const failed = selection.sourceId
    setAutoPickedSourceId(null)
    setSelection(null)
    auto.advance(failed)
  }, [selection, autoPickedSourceId, auto])
  let nativeState: BangumiSourceState = 'idle'
  if (releasesQuery.isFetching) nativeState = 'working'
  else if (releasesQuery.isError) nativeState = 'failed'
  else if (releasesQuery.isSuccess) {
    nativeState = releases.length > 0 ? 'ready' : 'empty'
  }

  return (
    <div className='min-h-dvh bg-background'>
      <header className='bg-background/95 supports-[backdrop-filter]:bg-background/80 sticky top-0 z-30 border-b backdrop-blur'>
        <div className='mx-auto flex max-w-7xl items-center gap-3 px-4 py-3'>
          <Button
            variant='ghost'
            size='sm'
            className='shrink-0 gap-1.5'
            onClick={() => props.onBack()}
          >
            <ArrowLeft className='size-4' />
            {t('Back')}
          </Button>
          <h1 className='min-w-0 flex-1 truncate text-base font-semibold' title={props.title}>
            {props.title}
          </h1>
        </div>
      </header>

      <div className='mx-auto max-w-7xl px-4 py-6'>
        <div className='grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)]'>
          <div className='min-w-0 space-y-4'>
            {auto.state === 'working' && !streamUrl && (
              <p
                role='status'
                className='text-muted-foreground text-xs'
              >
                {t('Finding a source that plays…')}
              </p>
            )}
            {/* Only a subject with nothing playable at all earns a block this
                size, and even then it names what went wrong and offers the
                manual picker as the way out. */}
            {auto.state === 'unavailable' && !streamUrl && !nativeSelected && (
              <section className='space-y-3 rounded-xl border p-3'>
                <div className='space-y-1'>
                  <p className='text-sm font-medium'>
                    {t('No source could be started automatically.')}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {auto.failure === 'network' &&
                      t(
                        'The instance could not reach the media sources. Check its network or proxy.'
                      )}
                    {auto.failure === 'blocked' &&
                      t(
                        'Every source it tried needs a check only a browser can pass. Pick one below and clear it.'
                      )}
                    {auto.failure === 'empty' &&
                      t(
                        'The sources answered but had nothing playable for this anime.'
                      )}
                  </p>
                </div>
                <div className='flex flex-wrap gap-2'>
                  <Button size='sm' onClick={openSourcePicker}>
                    {t('Choose a source')}
                  </Button>
                  <Button variant='outline' size='sm' onClick={auto.retry}>
                    {t('Retry')}
                  </Button>
                </div>
              </section>
            )}
            <BangumiPlayer
              streamUrl={streamUrl}
              format={selection?.format}
              title={title}
              comments={comments}
              progressKey={progressKey}
              onFailed={handlePlaybackFailed}
            />
            <div className='space-y-1' aria-live='polite'>
              {streamUrl && (
                <p className='text-sm font-semibold break-words'>{title}</p>
              )}
              {selection && (
                <p className='text-muted-foreground text-xs break-words'>
                  {t('Selected source: {{source}} · {{line}}', {
                    source: selection.sourceName,
                    line: selection.channelName,
                  })}
                </p>
              )}
              {nativeSelected && activeRelease && (
                <p
                  className='text-muted-foreground line-clamp-2 text-xs break-words'
                  title={activeRelease.title}
                >
                  {activeRelease.title}
                </p>
              )}
            </div>
            {danmakuQuery.isError && (
              <p className='text-muted-foreground flex flex-wrap items-center gap-2 text-xs'>
                {t('Could not load danmaku.')}
                <Button
                  variant='link'
                  size='sm'
                  className='h-auto p-0'
                  onClick={() => {
                    void danmakuQuery.refetch()
                  }}
                >
                  {t('Retry')}
                </Button>
              </p>
            )}
            {/* The episode list and the danmaku come from dandanplay, which is
                not what plays the video. When either fails the player keeps
                going and one line says what was lost, rather than a warning
                that reads as "nothing here works". */}
            {episodesQuery.isLoading && (
              <LoadingState inline message={t('Loading danmaku episodes…')} />
            )}
            {episodesQuery.isError && (
              <p className='text-muted-foreground flex flex-wrap items-center gap-2 text-xs'>
                {t('Could not load the danmaku episode list.')}
                <Button
                  variant='link'
                  size='sm'
                  className='h-auto p-0'
                  onClick={() => {
                    void episodesQuery.refetch()
                  }}
                >
                  {t('Retry')}
                </Button>
              </p>
            )}
            {episodesQuery.isSuccess && episodes.length === 0 && (
              <p className='text-muted-foreground text-xs'>
                {t('No danmaku is available for this anime.')}
              </p>
            )}
          {nativeSelected && episodes.length > 0 && (
            <section aria-label={t('Episodes')} className='space-y-2'>
              <h3 className='text-sm font-semibold'>{t('Episodes')}</h3>
              <ScrollArea className='max-h-48'>
                <div className='flex flex-wrap gap-2 p-1'>
                  {episodes.map((episode) => (
                    <Button
                      key={episode.episodeId}
                      size='lg'
                      className='min-w-9'
                      aria-label={episode.episodeTitle}
                      aria-pressed={
                        episode.episodeId === activeEpisode?.episodeId
                      }
                      variant={
                        episode.episodeId === activeEpisode?.episodeId
                          ? 'secondary'
                          : 'outline'
                      }
                      title={episode.episodeTitle}
                      onClick={() => {
                        setEpisodeId(episode.episodeId)
                        setReleaseTopicId(null)
                      }}
                    >
                      {episode.episodeNumber || episode.episodeTitle}
                    </Button>
                  ))}
                </div>
              </ScrollArea>
            </section>
          )}
          {selection && !nativeSelected && (
            <section
              aria-label={t('Match danmaku episode')}
              className='space-y-2'
            >
              <h3 className='text-sm font-semibold'>
                {t('Match danmaku episode')}
              </h3>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Select only the matching danmaku episode; source episode labels are not matched automatically.'
                )}
              </p>
              <ScrollArea className='max-h-48'>
                <div className='flex flex-wrap gap-2 p-1'>
                  <Button
                    variant={danmakuEpisodeId == null ? 'secondary' : 'outline'}
                    aria-pressed={danmakuEpisodeId == null}
                    onClick={() => setDanmakuEpisodeId(null)}
                  >
                    {t('No danmaku')}
                  </Button>
                  {episodes.map((episode) => (
                    <Button
                      key={episode.episodeId}
                      aria-label={t('Use danmaku for {{title}}', {
                        title: episode.episodeTitle,
                      })}
                      aria-pressed={danmakuEpisodeId === episode.episodeId}
                      variant={
                        danmakuEpisodeId === episode.episodeId
                          ? 'secondary'
                          : 'outline'
                      }
                      onClick={() => setDanmakuEpisodeId(episode.episodeId)}
                    >
                      {episode.episodeNumber || episode.episodeTitle}
                    </Button>
                  ))}
                </div>
              </ScrollArea>
            </section>
          )}
        </div>
        <aside
          ref={sourcesPanelRef}
          className='min-w-0 border-t pt-4 lg:border-t-0 lg:border-l lg:pt-0 lg:pl-4'
        >
          <BangumiMediaSources
            keyword={props.title}
            active
            nativeState={nativeState}
            nativeRequest={nativeRequest}
            onNativeSelect={selectManualNative}
            onSelectionChange={changeManualSelection}
            onPlay={playManualSource}
            openSourceId={pickerRequested ? auto.firstCandidateId : null}
            nativeMikan={
              <div className='space-y-3'>
                {releasesQuery.isLoading && (
                  <LoadingState message={t('Sources')} />
                )}
                {releasesQuery.isError && (
                  <ErrorState
                    className='min-h-0 p-3'
                    title={t('Could not load this source')}
                    onRetry={() => {
                      void releasesQuery.refetch()
                    }}
                  />
                )}
                {releasesQuery.isSuccess && releases.length === 0 && (
                  <EmptyState
                    className='min-h-0 p-3'
                    title={t('No sources found for this anime.')}
                  />
                )}
                {activeRelease && episodeNumber == null && (
                  <p className='text-muted-foreground text-xs'>
                    {t('Select a known episode before playing a season pack.')}
                  </p>
                )}
                {releases.length > 0 && (
                  <BangumiSourcePicker
                    key={activeEpisode?.episodeId}
                    releases={releases}
                    selectedId={activeRelease?.topicId}
                    episodeNumber={episodeNumber}
                    onSelect={(release) => setReleaseTopicId(release.topicId)}
                  />
                )}
              </div>
            }
          />
          </aside>
        </div>
      </div>
    </div>
  )
}
