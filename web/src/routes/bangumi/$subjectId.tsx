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
import { createFileRoute, useNavigate } from '@tanstack/react-router'

import { PublicLayout } from '@/components/layout'
import { ErrorState } from '@/components/error-state'
import { BangumiSubjectPage } from '@/features/bangumi/bangumi-subject-dialog'

export const Route = createFileRoute('/bangumi/$subjectId')({
  component: SubjectPage,
})

function SubjectPage() {
  const { subjectId } = Route.useParams()
  const navigate = useNavigate()
  const id = Number(subjectId)
  const valid = Number.isSafeInteger(id) && id > 0

  // The page owns the fetch: it is the only query for this subject, so a list
  // entry passed in does not double the requests. A direct link or a reload
  // still works because the page can render from the id alone.
  const back = () => {
    void navigate({ to: '/bangumi' })
  }

  if (!valid) {
    return (
      <PublicLayout backgroundMode='hero'>
        <div className='mx-auto w-full max-w-6xl px-3 pt-10 pb-10 sm:px-6 sm:pt-14 xl:px-8'>
          <ErrorState
            className='min-h-[200px]'
            title='This anime link is not valid.'
            onRetry={back}
          />
        </div>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout backgroundMode='hero'>
      <BangumiSubjectPage subjectId={id} onBack={back} />
    </PublicLayout>
  )
}