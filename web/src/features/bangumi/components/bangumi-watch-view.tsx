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
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'

import {
  bangumiTorrentUrl,
  getBangumiDanmaku,
  getBangumiEpisodes,
  getBangumiReleases,
} from '../api'
import { parseDanmaku } from '../lib/danmaku'
import { formatBytes } from '../lib/playback-state'
import type { BangumiRelease } from '../types'
import { BangumiPlayer } from './bangumi-player'

/**
 * Watching one anime: pick an episode, pick a release, play it with danmaku.
 *
 * The episode list comes from dandanplay and the releases from Mikan, and the
 * server has already matched a release to each episode where it could, so
 * opening an episode plays something immediately and choosing a different
 * release is the exception rather than the first step.
 */
export function BangumiWatchView(props: {
  subjectId: number
  title: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [episodeId, setEpisodeId] = useState<number | null>(null)
  const [releaseTopicId, setReleaseTopicId] = useState<string | null>(null)

  const episodesQuery = useQuery({
    queryKey: ['bangumi', 'episodes', props.subjectId],
    queryFn: () => getBangumiEpisodes(props.subjectId),
    enabled: props.open,
    staleTime: 60 * 60 * 1000,
    retry: false,
  })

  const releasesQuery = useQuery({
    queryKey: ['bangumi', 'releases', props.subjectId, props.title],
    queryFn: () => getBangumiReleases(props.subjectId, props.title),
    enabled: props.open,
    staleTime: 30 * 60 * 1000,
    retry: false,
  })

  const episodes = episodesQuery.data?.episodes ?? []
  const releases = releasesQuery.data?.releases ?? []

  const activeEpisode =
    episodes.find((episode) => episode.episodeId === episodeId) ??
    episodes[0] ??
    null

  const activeRelease =
    releases.find((release) => release.topicId === releaseTopicId) ??
    activeEpisode?.release ??
    releases[0] ??
    null

  const activeEpisodeId = activeEpisode?.episodeId

  const danmakuQuery = useQuery({
    queryKey: ['bangumi', 'danmaku', activeEpisodeId],
    queryFn: () => {
      if (activeEpisodeId == null) throw new Error('no episode selected')
      return getBangumiDanmaku(activeEpisodeId)
    },
    enabled: props.open && activeEpisodeId != null,
    staleTime: 10 * 60 * 1000,
    retry: false,
  })

  const comments = useMemo(
    () => parseDanmaku(danmakuQuery.data?.comments ?? []),
    [danmakuQuery.data]
  )

  const parsedEpisodeNumber = Number.parseInt(
    activeEpisode?.episodeNumber ?? '',
    10
  )
  const torrentUrl = activeRelease
    ? bangumiTorrentUrl(activeRelease.downloadLink)
    : null

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.title}
      contentClassName='sm:max-w-4xl'
    >
      <div className='space-y-4'>
        {episodesQuery.isLoading && <LoadingState />}

        {episodesQuery.isError && (
          <p className='text-destructive text-sm'>
            {t('Could not load the episodes.')}
          </p>
        )}

        {!episodesQuery.isLoading && !episodesQuery.isError && (
          <>
            <BangumiPlayer
              torrentUrl={torrentUrl}
              episodeNumber={
                Number.isFinite(parsedEpisodeNumber)
                  ? parsedEpisodeNumber
                  : undefined
              }
              comments={comments}
            />

            {releases.length === 0 && !releasesQuery.isLoading && (
              <p className='text-muted-foreground text-sm'>
                {t('No sources found for this anime.')}
              </p>
            )}

            <Separator />

            <section className='space-y-2'>
              <h3 className='text-sm font-medium'>{t('Episodes')}</h3>
              <div className='flex flex-wrap gap-1.5'>
                {episodes.map((episode) => (
                  <Button
                    key={episode.episodeId}
                    size='sm'
                    variant={
                      episode.episodeId === activeEpisode?.episodeId
                        ? 'secondary'
                        : 'ghost'
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
            </section>

            {releases.length > 0 && (
              <section className='space-y-2'>
                <h3 className='text-sm font-medium'>{t('Sources')}</h3>
                <ScrollArea className='h-40'>
                  <div className='space-y-1.5 pr-3'>
                    {releases.map((release) => (
                      <ReleaseOption
                        key={release.topicId}
                        release={release}
                        selected={release.topicId === activeRelease?.topicId}
                        onSelect={() => setReleaseTopicId(release.topicId)}
                      />
                    ))}
                  </div>
                </ScrollArea>
              </section>
            )}
          </>
        )}
      </div>
    </Dialog>
  )
}

/** One release, with the attributes that make two of them different. */
function ReleaseOption(props: {
  release: BangumiRelease
  selected: boolean
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const size = formatBytes(props.release.size)

  return (
    <button
      type='button'
      onClick={props.onSelect}
      aria-pressed={props.selected}
      className={cn(
        'hover:bg-muted/60 w-full rounded-md border p-2 text-left text-xs transition-colors',
        props.selected && 'border-primary bg-muted/40'
      )}
    >
      <span className='line-clamp-2'>{props.release.title}</span>
      <span className='text-muted-foreground mt-1 flex flex-wrap gap-x-2'>
        {props.release.resolution && <span>{props.release.resolution}</span>}
        {props.release.subtitleLanguages.length > 0 && (
          <span>{props.release.subtitleLanguages.join('/')}</span>
        )}
        {size && <span>{size}</span>}
        {props.release.matchKind !== 'EXACT' && (
          <span>{t('Unverified match')}</span>
        )}
      </span>
    </button>
  )
}
