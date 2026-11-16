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
import { useNavigate } from '@tanstack/react-router'
import { CalendarDays, Search, Tv } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  getBangumiCalendar,
  getBangumiTodayUpdates,
  searchBangumi,
} from './api'
import { BangumiCard } from './bangumi-card'
import { BangumiRanking } from './components/bangumi-ranking'
import type {
  BangumiCalendarDay,
  BangumiCalendarItem,
  BangumiRelease,
} from './types'

const SEARCH_DEBOUNCE_MS = 350

export function Bangumi(props: { keyword: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const submitted = props.keyword
  const [keyword, setKeyword] = useState(props.keyword)
  const [view, setView] = useState<'today' | 'calendar' | 'ranking'>(
    'today'
  )

  const calendarQuery = useQuery({
    queryKey: ['bangumi', 'calendar'],
    queryFn: getBangumiCalendar,
    staleTime: 5 * 60 * 1000,
    refetchInterval: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
    retry: false,
  })

  const todayQuery = useQuery({
    queryKey: ['bangumi', 'today'],
    queryFn: getBangumiTodayUpdates,
    staleTime: 5 * 60 * 1000,
    refetchInterval: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
    retry: false,
  })

  const searchQuery = useQuery({
    queryKey: ['bangumi', 'search', submitted],
    queryFn: () => searchBangumi(submitted),
    enabled: submitted.length > 0,
    staleTime: 10 * 60 * 1000,
    retry: false,
  })

  const debounceRef = useRef<number | null>(null)

  useEffect(() => {
    return () => {
      if (debounceRef.current !== null) {
        window.clearTimeout(debounceRef.current)
      }
    }
  }, [])

  // The committed term is written to the URL rather than kept in state, so the
  // search survives the round trip through a subject page. The input still
  // updates on every keystroke; only the committed value is reflected.
  const commitKeyword = (value: string) => {
    void navigate({
      to: '/bangumi',
      search: value ? { keyword: value } : {},
      replace: true,
    })
  }

  // Typing updates the input immediately but only searches once the user
  // pauses, so a title does not cost one upstream request per keystroke. Each
  // keystroke cancels the one before it: without that, clearing the field would
  // still be followed by the pending timer firing and searching the old text.
  const onKeywordChange = (value: string) => {
    setKeyword(value)
    if (debounceRef.current !== null) {
      window.clearTimeout(debounceRef.current)
      debounceRef.current = null
    }

    const trimmed = value.trim()
    if (!trimmed) {
      commitKeyword('')
      return
    }
    debounceRef.current = window.setTimeout(() => {
      debounceRef.current = null
      commitKeyword(trimmed)
    }, SEARCH_DEBOUNCE_MS)
  }

  const days = useMemo(() => {
    const raw = calendarQuery.data
    if (!raw) return []
    // The upstream answers with seven days starting today; keep only the days
    // that actually have a broadcast so the page opens on what is airing.
    return raw
      .map((day) => ({ ...day, items: day.items ?? [] }))
      .filter((day) => day.items.length > 0)
  }, [calendarQuery.data])

  const results = searchQuery.data?.data ?? []
  const searching = submitted.length > 0

  // The detail and player live on their own routes rather than in an overlay,
  // so opening one is a navigation the browser back button can undo.
  const openSubject = (item: BangumiCalendarItem) => {
    void navigate({
      to: '/bangumi/$subjectId',
      params: { subjectId: String(item.id) },
    })
  }

  return (
    <div className='mx-auto max-w-6xl space-y-8'>
      <div className='flex flex-wrap items-end justify-between gap-4'>
        <div className='space-y-1'>
          <div className='text-muted-foreground flex items-center gap-2 font-mono text-xs tracking-wider uppercase'>
            <Tv className='size-3.5' />
            {t('Anime')}
          </div>
          <h1 className='text-2xl font-semibold tracking-tight'>
            {t('Anime')}
          </h1>
        </div>
        <div className='relative w-full sm:w-72'>
          <Search
            aria-hidden
            className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2'
          />
          <Input
            value={keyword}
            onChange={(event) => onKeywordChange(event.target.value)}
            placeholder={t('Search anime')}
            aria-label={t('Search anime')}
            className='pl-8'
          />
        </div>
      </div>

      {searching ? (
        <SearchResults
          items={results}
          loading={searchQuery.isLoading}
          errored={searchQuery.isError}
          onSelect={openSubject}
        />
      ) : (
        <Tabs
          value={view}
          onValueChange={(value) => {
            if (
              value === 'today' ||
              value === 'calendar' ||
              value === 'ranking'
            ) {
              setView(value)
            }
          }}
        >
          <TabsList>
            <TabsTrigger value='today'>{t('Today')}</TabsTrigger>
            <TabsTrigger value='calendar'>{t('Broadcast calendar')}</TabsTrigger>
            <TabsTrigger value='ranking'>{t('Ranking')}</TabsTrigger>
          </TabsList>
          <TabsContent value='today'>
            <TodayUpdates
              releases={todayQuery.data?.releases ?? []}
              date={todayQuery.data?.date ?? ''}
              loading={todayQuery.isLoading}
              errored={todayQuery.isError}
            />
          </TabsContent>
          <TabsContent value='calendar'>
            <Calendar
              days={days}
              loading={calendarQuery.isLoading}
              errored={calendarQuery.isError}
              onSelect={openSubject}
            />
          </TabsContent>
          <TabsContent value='ranking'>
            <BangumiRanking onSelect={openSubject} />
          </TabsContent>
        </Tabs>
      )}
    </div>
  )
}

function Calendar(props: {
  days: BangumiCalendarDay[]
  loading: boolean
  errored: boolean
  onSelect: (item: BangumiCalendarItem) => void
}) {
  const { t } = useTranslation()

  if (props.loading) {
    return (
      <div className='grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5'>
        {Array.from({ length: 10 }, (_, index) => (
          <Skeleton key={index} className='h-52 w-full' />
        ))}
      </div>
    )
  }
  if (props.errored) {
    return (
      <ErrorState
        className='min-h-[200px]'
        title={t('Could not reach the anime source. Try again later.')}
      />
    )
  }
  if (props.days.length === 0) {
    return (
      <EmptyState
        className='min-h-[200px]'
        title={t('Nothing is airing today.')}
      />
    )
  }

  return (
    <div className='space-y-6'>
      {props.days.map((day) => (
        <section key={day.weekday.id} className='space-y-2'>
          <h2 className='text-muted-foreground flex items-center gap-1.5 text-sm font-medium'>
            <CalendarDays className='size-4' />
            {day.weekday.cn || day.weekday.en || day.weekday.id}
          </h2>
          <div className='grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5'>
            {day.items.map((item) => (
              <BangumiCard
                key={item.id}
                item={item}
                onSelect={() => props.onSelect(item)}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

function SearchResults(props: {
  items: BangumiCalendarItem[]
  loading: boolean
  errored: boolean
  onSelect: (item: BangumiCalendarItem) => void
}) {
  const { t } = useTranslation()

  if (props.loading) {
    return (
      <div className='grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5'>
        {Array.from({ length: 10 }, (_, index) => (
          <Skeleton key={index} className='h-52 w-full' />
        ))}
      </div>
    )
  }
  if (props.errored) {
    return (
      <ErrorState
        className='min-h-[200px]'
        title={t('Could not reach the anime source. Try again later.')}
      />
    )
  }
  if (props.items.length === 0) {
    return <EmptyState className='min-h-[200px]' title={t('No anime found.')} />
  }

  return (
    <div className='grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5'>
      {props.items.map((item) => (
        <BangumiCard
          key={item.id}
          item={item}
          onSelect={() => props.onSelect(item)}
        />
      ))}
    </div>
  )
}

function TodayUpdates(props: {
  releases: BangumiRelease[]
  date: string
  loading: boolean
  errored: boolean
}) {
  const { t } = useTranslation()

  if (props.loading) {
    return (
      <div className='space-y-2'>
        {Array.from({ length: 8 }, (_, index) => (
          <Skeleton key={index} className='h-16 w-full' />
        ))}
      </div>
    )
  }
  if (props.errored) {
    return (
      <ErrorState
        className='min-h-[200px]'
        title={t('Could not reach the anime source. Try again later.')}
      />
    )
  }
  if (props.releases.length === 0) {
    return (
      <EmptyState
        className='min-h-[200px]'
        title={t('Nothing has been released today yet.')}
      />
    )
  }

  return (
    <div className='space-y-3'>
      <div className='text-muted-foreground text-xs'>
        {t('{{count}} releases on {{date}}', {
          count: props.releases.length,
          date: props.date,
        })}
      </div>
      <ul className='divide-y overflow-hidden rounded-xl border'>
        {props.releases.map((release) => (
          <li
            key={`${release.topicId}:${release.title}`}
            className='flex min-w-0 flex-col gap-1 px-3 py-2.5 sm:flex-row sm:items-center sm:gap-3'
          >
            <span className='min-w-0 flex-1 truncate text-sm'>
              {release.title}
            </span>
            <span className='text-muted-foreground flex shrink-0 items-center gap-2 font-mono text-[11px]'>
              {release.episodeNumber > 0 && (
                <span>{t('EP {{n}}', { n: release.episodeNumber })}</span>
              )}
              {release.resolution && <span>{release.resolution}</span>}
              {release.alliance && (
                <span className='max-w-[10rem] truncate'>
                  {release.alliance}
                </span>
              )}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
