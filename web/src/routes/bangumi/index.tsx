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
import { createFileRoute } from '@tanstack/react-router'

import { PublicLayout } from '@/components/layout'
import { Bangumi } from '@/features/bangumi'
import { useRandomAnimeBackground } from '@/features/bangumi/hooks/use-random-anime-background'

/**
 * The search term lives in the URL so that opening a subject and coming back
 * returns to the same results. Component state would be gone by then, because
 * the calendar route remounts on the way back.
 */
type BangumiSearch = { keyword?: string }

export const Route = createFileRoute('/bangumi/')({
  validateSearch: (search: Record<string, unknown>): BangumiSearch => ({
    keyword: typeof search.keyword === 'string' ? search.keyword : undefined,
  }),
  component: AnimePage,
})

function AnimePage() {
  const { keyword } = Route.useSearch()
  const background = useRandomAnimeBackground()
  return (
    <PublicLayout
      backgroundMode='hero'
      backgroundOverride={background.url}
      backgroundPending={background.pending}
    >
      <div className='mx-auto w-full max-w-6xl px-3 pt-10 pb-10 sm:px-6 sm:pt-14 xl:px-8'>
        <Bangumi keyword={keyword ?? ''} />
      </div>
    </PublicLayout>
  )
}
