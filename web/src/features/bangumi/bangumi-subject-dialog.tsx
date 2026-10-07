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
import { ArrowLeft } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'

import { bangumiCoverUrl, getBangumiSubject } from './api'
import { BangumiWatchView } from './components/bangumi-watch-view'
import { bangumiTitle } from './lib/title'
import type { BangumiSubject } from './types'

/**
 * The upstream keys its infobox by an arbitrary label per entry, so a fixed set
 * of fields is not readable — a Chinese subject uses 中文名 and an English one
 * uses name. These are matched case-insensitively against whatever it sends.
 */
const INFOBOX_LABELS: { match: RegExp; labelKey: string }[] = [
  { match: /^(中文名|中文标题|Title)/i, labelKey: 'Chinese title' },
  { match: /^(日文名|Original title|原名)/i, labelKey: 'Original title' },
  { match: /^(放送开始|first aired|首播)/i, labelKey: 'First aired' },
  { match: /^(话数|集数|Episodes)/i, labelKey: 'Episodes' },
  { match: /^(导演|Director)/i, labelKey: 'Director' },
  { match: /^(制作|制作公司|Studio|Production)/i, labelKey: 'Studio' },
]

export function BangumiSubjectPage(props: {
  subjectId: number
  onBack: () => void
}) {
  const { t } = useTranslation()
  const [watching, setWatching] = useState(false)
  const id = props.subjectId

  const detailQuery = useQuery({
    queryKey: ['bangumi', 'subject', id],
    queryFn: () => getBangumiSubject(id),
    staleTime: 6 * 60 * 60 * 1000,
    retry: false,
    meta: { errorToast: false },
  })

  const detail = detailQuery.data
  const title = detail ? bangumiTitle(detail) : ''
  // 320 is twice the 160px the cover renders at; the host has no 320 and snaps
  // it up to 400, which stays sharp on a retina display.
  const cover = bangumiCoverUrl(detail?.images, 320)

  if (watching) {
    return (
      <BangumiWatchView
        key={id}
        subjectId={id}
        title={title}
        onBack={() => setWatching(false)}
      />
    )
  }

  return (
    <div className='min-h-dvh bg-background'>
      <header className='bg-background/95 supports-[backdrop-filter]:bg-background/80 sticky top-0 z-30 border-b backdrop-blur'>
        <div className='mx-auto flex max-w-6xl items-center gap-3 px-4 py-3'>
          <Button
            variant='ghost'
            size='sm'
            className='shrink-0 gap-1.5'
            onClick={props.onBack}
          >
            <ArrowLeft className='size-4' />
            {t('Back')}
          </Button>
          <h1
            className='min-w-0 flex-1 truncate text-base font-semibold'
            title={title}
          >
            {title}
          </h1>
          <Button
            className='shrink-0'
            disabled={!detail}
            onClick={() => setWatching(true)}
          >
            {t('Watch')}
          </Button>
        </div>
      </header>

      <div className='mx-auto max-w-6xl space-y-5 px-4 py-8'>
        {detailQuery.isLoading && (
          <div className='space-y-2'>
            <Skeleton className='h-4 w-2/3' />
            <Skeleton className='h-4 w-full' />
            <Skeleton className='h-4 w-5/6' />
          </div>
        )}

        {detailQuery.isError && (
          <ErrorState
            className='min-h-[160px]'
            title={t('Could not load this anime.')}
            onRetry={() => {
              void detailQuery.refetch()
            }}
          />
        )}

        {detail && (
          <div className='flex flex-col gap-5 sm:flex-row'>
            {cover && (
              <img
                src={cover}
                alt={title}
                className='h-auto w-full max-w-40 shrink-0 rounded-xl object-cover sm:w-36'
              />
            )}
            <div className='min-w-0 flex-1 space-y-4'>
              <div className='flex flex-wrap gap-2'>
                {typeof detail.rating?.score === 'number' && (
                  <StatusBadge
                    variant='success'
                    size='sm'
                    label={`${detail.rating.score.toFixed(1)} / 10`}
                  />
                )}
                {detail.total_episodes ? (
                  <StatusBadge
                    variant='neutral'
                    size='sm'
                    label={t('{{count}} episodes', {
                      count: detail.total_episodes,
                    })}
                  />
                ) : null}
                {detail.collection?.doing ? (
                  <StatusBadge
                    variant='info'
                    size='sm'
                    label={t('{{count}} watching', {
                      count: detail.collection.doing,
                    })}
                  />
                ) : null}
                {detail.collection?.collect ? (
                  <StatusBadge
                    variant='neutral'
                    size='sm'
                    label={t('{{count}} collected', {
                      count: detail.collection.collect,
                    })}
                  />
                ) : null}
              </div>

              {detail.summary && (
                <p className='text-sm leading-relaxed whitespace-pre-line'>
                  {detail.summary}
                </p>
              )}

              <SubjectFacts detail={detail} />

              {detail.tags && detail.tags.length > 0 && (
                <>
                  <Separator />
                  <div className='flex flex-wrap gap-1.5'>
                    {detail.tags.slice(0, 12).map((tag) => (
                      <StatusBadge
                        key={tag.name}
                        variant='neutral'
                        size='sm'
                        label={tag.name}
                      />
                    ))}
                  </div>
                </>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function SubjectFacts(props: { detail: BangumiSubject }) {
  const { t } = useTranslation()
  const facts = props.detail.infobox ?? []

  const rows = INFOBOX_LABELS.map(({ match, labelKey }) => {
    const entry = facts.find((item) => match.test(item.key ?? item.k ?? ''))
    const value = entry?.value ?? entry?.v
    return value ? { label: t(labelKey), value } : null
  }).filter((row): row is { label: string; value: string } => row !== null)

  if (rows.length === 0) return null

  return (
    <dl className='grid gap-x-4 gap-y-1.5 text-sm sm:grid-cols-2'>
      {rows.map((row) => (
        <div key={row.label} className='flex min-w-0 gap-2'>
          <dt className='text-muted-foreground shrink-0'>{row.label}</dt>
          <dd className='min-w-0 truncate' title={row.value}>
            {row.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}
