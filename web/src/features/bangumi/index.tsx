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
import { CalendarDays, Search, Tv } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'

import { getBangumiCalendar, searchBangumi } from './api'
import { BangumiCard } from './bangumi-card'
import { BangumiSubjectDialog } from './bangumi-subject-dialog'
import type { BangumiCalendarDay, BangumiCalendarItem } from './types'

const SEARCH_DEBOUNCE_MS = 350

export function Bangumi() {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [submitted, setSubmitted] = useState('')
  const [selected, setSelected] = useState<BangumiCalendarItem | null>(null)

  const calendarQuery = useQuery({
    queryKey: ['bangumi', 'calendar'],
    queryFn: getBangumiCalendar,
    staleTime: 30 * 60 * 1000,
    retry: false,
  })

  const searchQuery = useQuery({
    queryKey: ['bangumi', 'search', submitted],
    queryFn: () => searchBangumi(submitted),
    enabled: submitted.length > 0,
    staleTime: 10 * 60 * 1000,
    retry: false,
  })

  // Typing updates the input immediately but only searches once the user
  // pauses, so a title does not cost one upstream request per keystroke.
  const onKeywordChange = (value: string) => {
    setKeyword(value)
    const trimmed = value.trim()
    if (!trimmed) {
      setSubmitted('')
      return
    }
    window.setTimeout(() => setSubmitted(trimmed), SEARCH_DEBOUNCE_MS)
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

  return (
    <div className='mx-auto max-w-6xl space-y-6'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex items-center gap-2'>
          <Tv className='size-5' />
          <h1 className='text-lg font-semibold'>{t('Anime')}</h1>
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
          onSelect={setSelected}
        />
      ) : (
        <Calendar
          days={days}
          loading={calendarQuery.isLoading}
          errored={calendarQuery.isError}
          onSelect={setSelected}
        />
      )}

      <BangumiSubjectDialog
        subject={selected}
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) setSelected(null)
        }}
      />
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
      <p className='text-destructive text-sm'>
        {t('Could not reach the anime source. Try again later.')}
      </p>
    )
  }
  if (props.days.length === 0) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('Nothing is airing today.')}
      </p>
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
      <p className='text-destructive text-sm'>
        {t('Could not reach the anime source. Try again later.')}
      </p>
    )
  }
  if (props.items.length === 0) {
    return (
      <p className='text-muted-foreground text-sm'>{t('No anime found.')}</p>
    )
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
