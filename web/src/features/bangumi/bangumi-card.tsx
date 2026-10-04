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
import { Star } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { bangumiCoverUrl } from './api'
import type { BangumiCalendarItem } from './types'

/** The Chinese title when the subject has one, otherwise the original. */
export function bangumiTitle(item: BangumiCalendarItem): string {
  return item.name_cn?.trim() || item.name
}

export function BangumiCard(props: {
  item: BangumiCalendarItem
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const { item } = props
  const cover = bangumiCoverUrl(item.images)
  const score = item.rating?.score
  const title = bangumiTitle(item)

  return (
    <button
      type='button'
      onClick={props.onSelect}
      aria-label={t('View {{title}}', { title })}
      className='group bg-card focus-visible:outline-primary hover:border-foreground/25 block w-full overflow-hidden rounded-xl border text-left transition-colors focus-visible:outline-2 focus-visible:-outline-offset-2'
    >
      <div className='bg-muted relative aspect-2/3 w-full overflow-hidden'>
        {cover ? (
          <img
            src={cover}
            alt={title}
            loading='lazy'
            className='size-full object-cover transition-transform duration-200 group-hover:scale-[1.03]'
          />
        ) : (
          <div className='text-muted-foreground flex size-full items-center justify-center p-3 text-center text-xs'>
            {title}
          </div>
        )}
        {typeof score === 'number' && score > 0 && (
          <span className='absolute top-1.5 left-1.5 flex items-center gap-0.5 rounded-full bg-black/65 px-1.5 py-0.5 text-[11px] font-medium text-white'>
            <Star className='size-2.5 fill-amber-400 text-amber-400' />
            {score.toFixed(1)}
          </span>
        )}
      </div>
      <div className='space-y-0.5 p-2.5'>
        <p
          className='line-clamp-2 text-sm leading-snug font-medium'
          title={title}
        >
          {title}
        </p>
        {(item.air_date || item.summary) && (
          <p className='text-muted-foreground truncate text-xs'>
            {item.air_date || item.summary}
          </p>
        )}
      </div>
    </button>
  )
}
