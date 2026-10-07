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
import { ExternalLink, LibraryBig, ShieldAlert } from 'lucide-react'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Button, buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  getBangumiSources,
  getBangumiSourceSubject,
  resolveBangumiSource,
  searchBangumiSource,
} from '../api'
import type {
  BangumiMediaSelection,
  BangumiMediaSource,
  BangumiReleasesResponse,
  BangumiSourceChannel,
  BangumiSourceEpisode,
  BangumiSourceState,
  BangumiSourceStatus,
  BangumiSourceSubject,
} from '../types'

// Queue before Axios starts its timeout, rather than sending eighteen requests
// to the server's two-worker queue. Query cancellation also removes queued work.
class SourceRequestQueue {
  private pending: { start: () => void; cancel: () => void }[] = []
  private active = new Set<AbortController>()

  run<T>(
    signal: AbortSignal,
    request: (signal: AbortSignal) => Promise<T>,
    priority = false
  ): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const controller = new AbortController()
      let started = false
      const cancel = () => {
        controller.abort()
        if (!started) {
          this.pending = this.pending.filter((task) => task !== taskEntry)
          signal.removeEventListener('abort', cancel)
          reject(new DOMException('Source request cancelled', 'AbortError'))
        }
      }
      const taskEntry = {
        cancel,
        start: () => {
          started = true
          this.active.add(controller)
          void request(controller.signal)
            .then((data) => {
              controller.signal.throwIfAborted()
              resolve(data)
            })
            .catch(reject)
            .finally(() => {
              signal.removeEventListener('abort', cancel)
              this.active.delete(controller)
              this.startNext()
            })
        },
      }
      if (signal.aborted) {
        cancel()
        return
      }
      signal.addEventListener('abort', cancel, { once: true })
      if (priority) this.pending.unshift(taskEntry)
      else this.pending.push(taskEntry)
      this.startNext()
    })
  }

  cancel(): void {
    for (const task of this.pending) task.cancel()
    for (const controller of this.active) controller.abort()
  }

  private startNext(): void {
    while (this.active.size < 2 && this.pending.length > 0) {
      this.pending.shift()?.start()
    }
  }
}

function SourceStateLabel(props: { state: BangumiSourceState }) {
  const { t } = useTranslation()
  let label: string
  switch (props.state) {
    case 'working':
      label = t('Searching source…')
      break
    case 'ready':
      label = t('Source results ready')
      break
    case 'empty':
      label = t('No source results')
      break
    case 'failed':
      label = t('Source query failed')
      break
    case 'captcha_required':
      label = t('CAPTCHA required')
      break
    case 'rate_limited':
      label = t('Source rate limited')
      break
    case 'browser_required':
      label = t('Browser required')
      break
    case 'invalid_config':
      label = t('Invalid source configuration')
      break
    case 'selection_required':
      label = t('Choose a torrent file')
      break
    default:
      label = t('Source waiting')
      break
  }
  const blocked = ['captcha_required', 'failed', 'invalid_config'].includes(
    props.state
  )
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center gap-1 text-xs',
        blocked ? 'text-destructive' : 'text-muted-foreground'
      )}
    >
      {props.state === 'captcha_required' && (
        <ShieldAlert aria-hidden className='size-3.5' />
      )}
      {label}
    </span>
  )
}

function SourceIcon(props: { source: BangumiMediaSource }) {
  const [failed, setFailed] = useState(false)
  // Remote catalog icons are not fetched from the visitor's browser. A host
  // may supply a same-origin admitted icon; otherwise use the library icon.
  const localIcon =
    props.source.icon.startsWith('/') &&
    !props.source.icon.startsWith('//') &&
    !props.source.icon.includes('\\')
  if (localIcon && !failed) {
    return (
      <img
        src={props.source.icon}
        alt=''
        referrerPolicy='no-referrer'
        className='size-5 shrink-0 rounded-md object-contain'
        onError={() => setFailed(true)}
      />
    )
  }
  return (
    <LibraryBig aria-hidden className='text-muted-foreground size-5 shrink-0' />
  )
}

function SourceFeedback(props: {
  source: BangumiMediaSource
  status: BangumiSourceStatus
  onClearance: (value: string) => void
  onRetry: () => void
}) {
  const { t } = useTranslation()
  const inputId = useId()
  const [draft, setDraft] = useState('')
  const submitClearance = () => {
    const value = draft.trim()
    if (!value) return
    props.onClearance(value)
    setDraft('')
    props.onRetry()
  }
  if (props.status.state === 'empty') {
    return (
      <EmptyState
        className='min-h-0 py-4'
        title={t('No sources found for this anime.')}
        action={
          <Button variant='outline' size='sm' onClick={props.onRetry}>
            {t('Retry')}
          </Button>
        }
      />
    )
  }
  return (
    <div className='space-y-2'>
      <ErrorState
        className='min-h-0 py-4'
        title={t('Could not load this source')}
        description={props.status.message}
        onRetry={props.onRetry}
      />
      {props.status.state === 'captcha_required' && (
        <div className='space-y-2 rounded-xl border p-2'>
          <p className='text-muted-foreground text-xs'>
            {t(
              'This source is asking the server for a check only a browser can pass. Open the site, finish the check there, then paste the cookie it sets to continue.'
            )}
          </p>
          <div className='flex flex-wrap items-center gap-2'>
            {props.source.websiteUrl && (
              <a
                href={props.source.websiteUrl}
                target='_blank'
                rel='noreferrer noopener'
                className={buttonVariants({ variant: 'outline', size: 'sm' })}
              >
                <ExternalLink aria-hidden className='size-4' />
                {t('Open {{name}}', { name: props.source.name })}
              </a>
            )}
          </div>
          <div className='space-y-1'>
            <Label htmlFor={inputId} className='text-xs'>
              {t('Clearance cookie')}
            </Label>
            <div className='flex gap-2'>
              <Input
                id={inputId}
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                placeholder='cf_clearance=…'
                autoComplete='off'
                spellCheck={false}
              />
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={!draft.trim()}
                onClick={submitClearance}
              >
                {t('Verify and retry')}
              </Button>
            </div>
          </div>
        </div>
      )}
      {props.status.state === 'browser_required' && (
        <p className='text-muted-foreground text-xs'>
          {t('This source requires a browser on the server.')}
        </p>
      )}
      {props.status.retryAt && (
        <p className='text-muted-foreground text-xs'>
          {t('Retry after {{time}}', {
            time: formatTimestampToDate(
              Date.parse(props.status.retryAt),
              'milliseconds'
            ),
          })}
        </p>
      )}
    </div>
  )
}

function SourceResolution(props: {
  source: BangumiMediaSource
  subject: BangumiSourceSubject
  channel: BangumiSourceChannel
  episode: BangumiSourceEpisode
  queue: SourceRequestQueue
  clearance?: string
  onClearance: (value: string) => void
  onStateChange: (state: BangumiSourceState | null) => void
  onPlay: (selection: BangumiMediaSelection) => void
}) {
  const { t, i18n } = useTranslation()
  const [fileId, setFileId] = useState<string | null>(null)
  const query = useQuery({
    queryKey: [
      'bangumi',
      'sources',
      props.source.id,
      'resolve',
      props.episode.id,
      fileId,
      props.clearance,
    ],
    queryFn: ({ signal }) =>
      props.queue.run(
        signal,
        (requestSignal) =>
          resolveBangumiSource(
            props.source.id,
            props.episode.id,
            requestSignal,
            fileId ?? undefined,
            props.clearance
          ),
        true
      ),
    retry: false,
    staleTime: 0,
    meta: { errorToast: false },
  })
  const streamUrl =
    query.data?.state === 'ready' ? query.data.streamUrl : undefined
  const format = query.data?.format
  const { source, subject, channel, episode, onPlay, onStateChange } = props
  let resolutionState: BangumiSourceState = query.data?.state ?? 'idle'
  if (query.isFetching) resolutionState = 'working'
  else if (query.isError) resolutionState = 'failed'
  useEffect(() => {
    onStateChange(resolutionState)
    return () => onStateChange(null)
  }, [resolutionState, onStateChange])
  useEffect(() => {
    if (!streamUrl) return
    onPlay({
      sourceId: source.id,
      sourceName: source.name,
      subjectName: subject.name,
      channelId: channel.id,
      channelName: channel.label || channel.name,
      episode,
      streamUrl,
      format,
    })
  }, [streamUrl, format, source, subject, channel, episode, onPlay])

  if (query.isFetching) {
    return <LoadingState inline message={t('Resolving episode…')} />
  }
  if (query.isError) {
    return (
      <SourceFeedback
        source={props.source}
        status={{ sourceId: props.source.id, state: 'failed' }}
        onClearance={props.onClearance}
        onRetry={() => {
          void query.refetch()
        }}
      />
    )
  }
  if (!query.data) return null
  if (query.data.state === 'selection_required' && query.data.files?.length) {
    return (
      <section aria-label={t('Choose a torrent file')} className='space-y-2'>
        <h4 className='text-sm font-medium'>{t('Choose a torrent file')}</h4>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Select the file for this episode; season packs are not selected automatically.'
          )}
        </p>
        <div className='flex flex-col gap-2'>
          {query.data.files.map((file) => (
            <Button
              key={file.id}
              variant='outline'
              className='h-auto min-h-9 justify-start whitespace-normal'
              aria-pressed={fileId === file.id}
              onClick={() => setFileId(file.id)}
            >
              <span className='min-w-0 flex-1 break-all'>{file.name}</span>
              <span className='text-muted-foreground shrink-0 text-xs'>
                {t('{{size}} bytes', {
                  size: formatNumber(file.size, toIntlLocale(i18n.language)),
                })}
              </span>
            </Button>
          ))}
        </div>
      </section>
    )
  }
  if (query.data.state !== 'ready') {
    return (
      <SourceFeedback
        source={props.source}
        status={query.data}
        onClearance={props.onClearance}
        onRetry={() => {
          void query.refetch()
        }}
      />
    )
  }
  if (!streamUrl) {
    return (
      <ErrorState
        className='min-h-0 py-4'
        title={t('No admitted media stream is available.')}
        onRetry={() => {
          void query.refetch()
        }}
      />
    )
  }
  return (
    <p role='status' className='text-muted-foreground text-xs'>
      {t('Source resolved; browser compatibility is not verified.')}
    </p>
  )
}

function ManagedSourceRow(props: {
  source: BangumiMediaSource
  keyword: string
  selected: boolean
  queue: SourceRequestQueue
  clearance?: string
  onClearance: (value: string) => void
  onSelectionChange: () => void
  onPlay: (selection: BangumiMediaSelection) => void
}) {
  const { t } = useTranslation()
  const [subjectId, setSubjectId] = useState<string | null>(null)
  const [channelId, setChannelId] = useState<string | null>(null)
  const [episodeId, setEpisodeId] = useState<string | null>(null)
  const [resolutionState, setResolutionState] =
    useState<BangumiSourceState | null>(null)
  const search = useQuery({
    queryKey: [
      'bangumi',
      'sources',
      props.source.id,
      'search',
      props.keyword,
      props.clearance,
    ],
    queryFn: ({ signal }) =>
      props.queue.run(signal, (requestSignal) =>
        searchBangumiSource(
          props.source.id,
          props.keyword,
          requestSignal,
          props.clearance
        )
      ),
    enabled: props.source.enabled && props.keyword.length > 0,
    retry: false,
    staleTime: 5 * 60 * 1000,
    meta: { errorToast: false },
  })
  const subjects = search.data?.state === 'ready' ? search.data.subjects : []
  const subject =
    subjects.find((item) => item.id === subjectId) ??
    (subjects.length === 1 ? subjects[0] : undefined)
  const browse = useQuery({
    queryKey: [
      'bangumi',
      'sources',
      props.source.id,
      'subject',
      subject?.id,
      props.clearance,
    ],
    queryFn: ({ signal }) => {
      if (!subject) throw new Error('No source subject selected')
      return props.queue.run(
        signal,
        (requestSignal) =>
          getBangumiSourceSubject(
            props.source.id,
            subject.id,
            requestSignal,
            props.clearance
          ),
        props.selected
      )
    },
    enabled: props.source.enabled && subject != null,
    retry: false,
    staleTime: 5 * 60 * 1000,
    meta: { errorToast: false },
  })
  const channels = browse.data?.state === 'ready' ? browse.data.channels : []
  const channel = channels.find((item) => item.id === channelId)
  const episode = channel?.episodes.find((item) => item.id === episodeId)
  let state: BangumiSourceState = 'idle'
  let status: BangumiSourceStatus | undefined = subject
    ? browse.data
    : search.data
  if (search.isFetching || browse.isFetching) state = 'working'
  else if (search.isError || browse.isError) state = 'failed'
  else if (status) state = status.state
  if (
    state === 'ready' &&
    ((subject && channels.length === 0) || subjects.length === 0)
  ) {
    state = 'empty'
  }
  if (search.isError || browse.isError) {
    status = { sourceId: props.source.id, state: 'failed' }
  } else if (state === 'empty') {
    status = { ...status, sourceId: props.source.id, state }
  }
  const retry = () => {
    if (subject) void browse.refetch()
    else void search.refetch()
  }

  return (
    <AccordionItem value={props.source.id}>
      <AccordionTrigger
        aria-label={props.source.name}
        className='items-center gap-2 hover:no-underline'
      >
        <SourceIcon source={props.source} />
        <span className='min-w-0 flex-1 break-words'>{props.source.name}</span>
        {props.source.enabled ? (
          <SourceStateLabel
            state={
              props.selected && episode && resolutionState
                ? resolutionState
                : state
            }
          />
        ) : (
          <span className='text-muted-foreground text-xs'>
            {t('Source disabled')}
          </span>
        )}
      </AccordionTrigger>
      <AccordionContent>
        <div className='space-y-3 px-1'>
          {!props.source.enabled && (
            <EmptyState className='min-h-0 py-4' title={t('Source disabled')} />
          )}
          {state === 'working' && (
            <LoadingState inline message={t('Searching source…')} />
          )}
          {props.source.enabled &&
            ['idle', 'working', 'ready'].includes(state) && (
              <Button
                variant='ghost'
                size='sm'
                disabled={search.isFetching || browse.isFetching}
                onClick={retry}
              >
                {t('Retry')}
              </Button>
            )}
          {status && !['idle', 'working', 'ready'].includes(state) && (
            <SourceFeedback
              key={props.source.id}
              source={props.source}
              status={status}
              onClearance={props.onClearance}
              onRetry={retry}
            />
          )}
          {subjects.length > 1 && (
            <section
              aria-label={t('Choose a source subject')}
              className='space-y-2'
            >
              <h4 className='text-sm font-medium'>
                {t('Choose a source subject')}
              </h4>
              <div className='flex flex-wrap gap-2'>
                {subjects.map((item) => (
                  <Button
                    key={item.id}
                    variant={subjectId === item.id ? 'secondary' : 'outline'}
                    className='h-auto min-h-9 max-w-full whitespace-normal'
                    aria-pressed={subjectId === item.id}
                    onClick={() => {
                      if (subjectId === item.id) return
                      props.onSelectionChange()
                      setSubjectId(item.id)
                      setChannelId(null)
                      setEpisodeId(null)
                    }}
                  >
                    {item.name}
                  </Button>
                ))}
              </div>
            </section>
          )}
          {channels.length > 0 && (
            <section aria-label={t('Source lines')} className='space-y-3'>
              <p className='text-muted-foreground text-xs'>{subject?.name}</p>
              <div className='flex flex-wrap gap-2'>
                {channels.map((item) => (
                  <Button
                    key={item.id}
                    variant={channelId === item.id ? 'secondary' : 'outline'}
                    aria-pressed={channelId === item.id}
                    onClick={() => {
                      if (channelId === item.id) return
                      props.onSelectionChange()
                      setChannelId(item.id)
                      setEpisodeId(null)
                    }}
                  >
                    {item.label || item.name || t('Unnamed line')}
                  </Button>
                ))}
              </div>
              {channel && (
                <section
                  aria-label={t('Source episodes')}
                  className='space-y-2'
                >
                  {channel.episodes.length === 0 && (
                    <EmptyState
                      className='min-h-0 py-4'
                      title={t('No episodes available.')}
                    />
                  )}
                  <div className='flex flex-wrap gap-2'>
                    {channel.episodes.map((item) => (
                      <Button
                        key={item.id}
                        variant={
                          episodeId === item.id && props.selected
                            ? 'secondary'
                            : 'outline'
                        }
                        aria-label={item.name || item.episodeNumber}
                        title={item.name}
                        aria-pressed={episodeId === item.id && props.selected}
                        onClick={() => {
                          if (episodeId === item.id && props.selected) return
                          props.onSelectionChange()
                          setEpisodeId(item.id)
                        }}
                      >
                        {item.episodeNumber || item.name}
                      </Button>
                    ))}
                  </div>
                </section>
              )}
            </section>
          )}
          {props.selected && subject && channel && episode && (
            <SourceResolution
              key={`${subject.id}:${channel.id}:${episode.id}`}
              source={props.source}
              subject={subject}
              channel={channel}
              episode={episode}
              queue={props.queue}
              clearance={props.clearance}
              onClearance={props.onClearance}
              onStateChange={setResolutionState}
              onPlay={props.onPlay}
            />
          )}
        </div>
      </AccordionContent>
    </AccordionItem>
  )
}

export type NativeMikanRequest = (
  signal: AbortSignal
) => Promise<BangumiReleasesResponse>

type MediaSourcesProps = {
  keyword: string
  active: boolean
  nativeMikan: ReactNode
  nativeRequest: NativeMikanRequest
  nativeState: BangumiSourceState
  /**
   * A source to open on request, set when the page hands the viewer here after
   * an automatic pick failed. It opens the row so its subjects are visible
   * rather than only scrolling the panel into view.
   */
  openSourceId?: string | null
  onNativeSelect: (loadReleases: NativeMikanRequest) => void
  onSelectionChange: () => void
  onPlay: (selection: BangumiMediaSelection) => void
}

function SourceSession(
  props: MediaSourcesProps & {
    sources: BangumiMediaSource[]
    legacyMikanSourceId: string
    kind: 'web' | 'bt'
  }
) {
  const { t } = useTranslation()
  const [queue] = useState(() => new SourceRequestQueue())
  const [selectedId, setSelectedId] = useState<string | null>(null)
  // A clearance the viewer pasted for one source. It is held here rather than in
  // the row so it survives the row's re-mounts, and it is sent with the retry
  // that follows; the server keeps it only briefly, so nothing is persisted.
  const [clearances, setClearances] = useState<Record<string, string>>({})
  const openedSourceRef = useRef<string | null>(null)
  useEffect(() => () => queue.cancel(), [queue])
  // Open the requested row once. It is idempotent so a re-render of the panel
  // does not re-open it and clear a selection the viewer has since made.
  useEffect(() => {
    const target = props.openSourceId
    if (!target || openedSourceRef.current === target) return
    openedSourceRef.current = target
    setSelectedId(target)
    if (
      target === props.legacyMikanSourceId &&
      props.sources.find((source) => source.id === target)?.enabled
    ) {
      props.onNativeSelect((signal) =>
        queue.run(signal, props.nativeRequest, true)
      )
    }
  }, [
    // The guard above makes this idempotent, so re-running on any prop change
    // costs a comparison and never re-opens the row.
    props,
    queue,
  ])
  const sources = props.sources.filter((source) => source.kind === props.kind)
  if (sources.length === 0) {
    return (
      <EmptyState
        className='min-h-0 py-6'
        title={t('No sources found for this anime.')}
      />
    )
  }
  return (
    <ScrollArea className='h-[28rem] max-h-[60dvh] lg:max-h-[calc(100dvh-16rem)]'>
      <Accordion
        value={selectedId ? [selectedId] : []}
        onValueChange={(value) => {
          const next = typeof value[0] === 'string' ? value[0] : null
          setSelectedId(next)
          props.onSelectionChange()
          if (
            next === props.legacyMikanSourceId &&
            sources.find((source) => source.id === next)?.enabled
          ) {
            props.onNativeSelect((signal) =>
              queue.run(signal, props.nativeRequest, true)
            )
          }
        }}
      >
        {sources.map((source) =>
          source.id === props.legacyMikanSourceId ? (
            <AccordionItem key={source.id} value={source.id}>
              <AccordionTrigger
                aria-label={source.name}
                className='items-center gap-2 hover:no-underline'
              >
                <SourceIcon source={source} />
                <span className='min-w-0 flex-1 break-words'>
                  {source.name}
                </span>
                {source.enabled ? (
                  <SourceStateLabel state={props.nativeState} />
                ) : (
                  <span className='text-muted-foreground text-xs'>
                    {t('Source disabled')}
                  </span>
                )}
              </AccordionTrigger>
              <AccordionContent>
                {selectedId === source.id &&
                  (source.enabled ? (
                    props.nativeMikan
                  ) : (
                    <EmptyState
                      className='min-h-0 py-4'
                      title={t('Source disabled')}
                    />
                  ))}
              </AccordionContent>
            </AccordionItem>
          ) : (
            <ManagedSourceRow
              key={source.id}
              source={source}
              keyword={props.keyword}
              selected={selectedId === source.id}
              queue={queue}
              clearance={clearances[source.id]}
              onClearance={(value) =>
                setClearances((prev) => ({ ...prev, [source.id]: value }))
              }
              onSelectionChange={props.onSelectionChange}
              onPlay={props.onPlay}
            />
          )
        )}
      </Accordion>
    </ScrollArea>
  )
}

export function BangumiMediaSources(props: MediaSourcesProps) {
  if (!props.active) return null
  return <MediaSources key={props.keyword} {...props} />
}

function MediaSources(props: MediaSourcesProps) {
  const { t, i18n } = useTranslation()
  const inputId = useId()
  const [draft, setDraft] = useState(props.keyword ?? '')
  const [keyword, setKeyword] = useState(props.keyword ?? '')
  const [kind, setKind] = useState<'web' | 'bt'>('web')
  const catalog = useQuery({
    queryKey: ['bangumi', 'sources'],
    // No AbortSignal: the catalog is an embedded file the server answers without
    // reaching any upstream, so there is nothing to cancel. Handing it a signal
    // only made the query fail when the panel remounted -- the dialog renders
    // with the list title first and re-renders with the full one, and that
    // remount aborted the in-flight request and showed the catalog as broken.
    queryFn: () => getBangumiSources(),
    enabled: props.active,
    staleTime: 5 * 60 * 1000,
    meta: { errorToast: false },
  })
  const sources = catalog.data?.sources ?? []
  const locale = toIntlLocale(i18n.language)
  return (
    <section aria-label={t('Media sources')} className='min-w-0 space-y-3'>
      <form
        className='space-y-2'
        onSubmit={(event) => {
          event.preventDefault()
          const next = draft.trim()
          if (!next || next === keyword) return
          props.onSelectionChange()
          setKeyword(next)
        }}
      >
        <Label htmlFor={inputId}>{t('Search source titles')}</Label>
        <div className='flex gap-2'>
          <Input
            id={inputId}
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
          />
          <Button type='submit' variant='outline' disabled={!draft.trim()}>
            {t('Search')}
          </Button>
        </div>
      </form>
      {catalog.isLoading && <LoadingState message={t('Media sources')} />}
      {catalog.isError && (
        <ErrorState
          className='min-h-0 py-4'
          title={t('Could not load the source catalog.')}
          onRetry={() => {
            void catalog.refetch()
          }}
        />
      )}
      {catalog.isSuccess && (
        <Tabs
          value={kind}
          onValueChange={(value) => {
            if (value !== 'web' && value !== 'bt') return
            props.onSelectionChange()
            setKind(value)
          }}
        >
          <TabsList className='w-full'>
            <TabsTrigger value='web'>
              {t('Website sources')}{' '}
              <span className='text-muted-foreground text-xs'>
                {formatNumber(
                  sources.filter((source) => source.kind === 'web').length,
                  locale
                )}
              </span>
            </TabsTrigger>
            <TabsTrigger value='bt'>
              {t('BitTorrent sources')}{' '}
              <span className='text-muted-foreground text-xs'>
                {formatNumber(
                  sources.filter((source) => source.kind === 'bt').length,
                  locale
                )}
              </span>
            </TabsTrigger>
          </TabsList>
          <p className='text-muted-foreground text-xs'>
            {t('Source results do not guarantee browser playback.')}
          </p>
          <TabsContent value={kind}>
            {props.active && (
              <SourceSession
                key={`${kind}:${keyword}`}
                {...props}
                keyword={keyword}
                sources={sources}
                legacyMikanSourceId={catalog.data.legacyMikanSourceId}
                kind={kind}
              />
            )}
          </TabsContent>
        </Tabs>
      )}
    </section>
  )
}
