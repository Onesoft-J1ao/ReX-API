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
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'

import { bangumiCoverUrl, getBangumiSubject } from './api'
import { bangumiTitle } from './bangumi-card'
import type { BangumiCalendarItem, BangumiSubject } from './types'

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

export function BangumiSubjectDialog(props: {
  subject: BangumiCalendarItem | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const id = props.subject?.id

  const detailQuery = useQuery({
    queryKey: ['bangumi', 'subject', id],
    queryFn: () => getBangumiSubject(id!),
    enabled: props.open && id != null,
    staleTime: 6 * 60 * 60 * 1000,
    retry: false,
  })

  const detail = detailQuery.data?.data
  // The list entry already carries a title and a cover, so the dialog opens
  // populated and fills in the rest once the full record lands.
  const title = detail
    ? bangumiTitle(detail)
    : props.subject
      ? bangumiTitle(props.subject)
      : ''
  const cover = bangumiCoverUrl(detail?.images ?? props.subject?.images, 'large')

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={title}
      description={detail?.date ? t('Aired from {{date}}', { date: detail.date }) : undefined}
      contentClassName='sm:max-w-3xl'
    >
      {detailQuery.isLoading && (
        <div className='space-y-2'>
          <Skeleton className='h-4 w-2/3' />
          <Skeleton className='h-4 w-full' />
          <Skeleton className='h-4 w-5/6' />
        </div>
      )}

      {detailQuery.isError && (
        <p className='text-destructive text-sm'>{t('Could not load this anime.')}</p>
      )}

      {detail && (
        <div className='flex flex-col gap-5 sm:flex-row'>
          {cover && (
            <img
              src={cover}
              alt={title}
              className='h-auto w-full max-w-40 shrink-0 rounded-lg object-cover sm:w-36'
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
                  label={t('{{count}} episodes', { count: detail.total_episodes })}
                />
              ) : null}
              {detail.collection?.doing ? (
                <StatusBadge
                  variant='info'
                  size='sm'
                  label={t('{{count}} watching', { count: detail.collection.doing })}
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
    </Dialog>
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