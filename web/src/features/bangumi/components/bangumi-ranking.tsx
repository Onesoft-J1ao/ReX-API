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
import { Trophy } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

import { getBangumiRanking } from '../api'
import { BangumiCard } from '../bangumi-card'
import type { BangumiCalendarItem } from '../types'

// The server caps how deep a caller may page, but the ranking is only useful
// near the top, so the picker stops well short of it. Capping here keeps the
// bound visible instead of relying on the server's silent clamp.
const RANKING_MAX_PAGE = 20

export function BangumiRanking(props: {
  onSelect: (item: BangumiCalendarItem) => void
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: ['bangumi', 'ranking', page],
    queryFn: () => getBangumiRanking(page),
    staleTime: 5 * 60 * 1000,
    refetchInterval: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
    retry: false,
  })
  const items = query.data?.data ?? []
  // The page size comes from the response rather than a second constant: the
  // rank of the first card is only correct if it agrees with the server's.
  const pageSize = query.data?.limit || items.length
  const total = query.data?.total ?? 0
  const maxPage = Math.max(1, Math.min(RANKING_MAX_PAGE, Math.ceil(total / (pageSize || 1))))

  return (
    <section aria-label={t('Ranking')} className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h2 className='flex items-center gap-1.5 text-sm font-medium'>
          <Trophy aria-hidden className='size-4 text-amber-500' />
          {t('Ranking')}
        </h2>
        <span className='text-muted-foreground text-xs'>
          {t('Ranked by the anime source, not this instance.')}
        </span>
      </div>

      {query.isLoading && (
        <div className='grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6'>
          {Array.from({ length: 12 }, (_, index) => (
            <Skeleton key={index} className='h-52 w-full' />
          ))}
        </div>
      )}
      {query.isError && (
        <ErrorState
          className='min-h-[200px]'
          title={t('Could not reach the anime source. Try again later.')}
          onRetry={() => {
            void query.refetch()
          }}
        />
      )}
      {query.isSuccess && items.length === 0 && (
        <EmptyState
          className='min-h-[200px]'
          title={t('No anime found.')}
        />
      )}
      {query.isSuccess && items.length > 0 && (
        <div className='grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6'>
          {items.map((item, index) => (
            <BangumiCard
              key={item.id}
              item={item}
              rank={(page - 1) * pageSize + index + 1}
              onSelect={() => props.onSelect(item)}
            />
          ))}
        </div>
      )}

      {query.isSuccess && (
        <nav
          aria-label={t('Ranking pages')}
          className='flex items-center justify-center gap-3'
        >
          <Button
            variant='outline'
            size='sm'
            disabled={page <= 1 || query.isFetching}
            onClick={() => setPage((value) => Math.max(1, value - 1))}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-sm tabular-nums'>
            {t('Page {{page}}', { page })}
          </span>
          <Button
            variant='outline'
            size='sm'
            disabled={page >= maxPage || query.isFetching}
            onClick={() => setPage((value) => Math.min(maxPage, value + 1))}
          >
            {t('Next')}
          </Button>
        </nav>
      )}
    </section>
  )
}
