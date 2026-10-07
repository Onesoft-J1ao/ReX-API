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
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { BackgroundCredit as BackgroundCreditData } from '@/hooks/use-unsplash-background'

/**
 * The credit the Unsplash API guidelines require when their images are shown:
 * the photographer's name and Unsplash both linked. Rendered as a quiet line
 * pinned to the bottom of the backdrop so it never competes with the page.
 */
export function BackgroundCredit(props: {
  credit: BackgroundCreditData
  /** The landing hero sits on a dark photo, so it overrides the tone. */
  className?: string
}) {
  const { t } = useTranslation()

  return (
    <div
      className={cn(
        'absolute inset-x-0 bottom-0 z-10 px-4 pb-2 text-right text-[10px] leading-4 sm:px-6',
        props.className ?? 'text-muted-foreground/70'
      )}
    >
      <span>
        {t('Photo by')}{' '}
        <a
          href={props.credit.authorLink}
          target='_blank'
          rel='noopener noreferrer'
          className='underline underline-offset-2 transition-colors hover:opacity-80'
        >
          {props.credit.authorName}
        </a>{' '}
        {t('on Unsplash')}
        <a
          href='https://unsplash.com/?utm_source=rex_api&utm_medium=referral'
          target='_blank'
          rel='noopener noreferrer'
          className='underline underline-offset-2 transition-colors hover:opacity-80'
        >
          Unsplash
        </a>
      </span>
    </div>
  )
}
