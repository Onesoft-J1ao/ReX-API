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
import { z } from 'zod'

import { PublicLayout } from '@/components/layout'
import { PriceSentinelEditor } from '@/features/price-sentinel/components/price-sentinel-editor'

const searchSchema = z.object({
  token: z.string().catch(''),
})

export const Route = createFileRoute('/price-sentinel')({
  component: PriceSentinelPage,
  validateSearch: searchSchema,
})

/**
 * The page behind the sentinel's emailed link. It sits outside the
 * authenticated shell on purpose: the operator arrives from an email and
 * authenticates by pasting an admin access token, which may well be on a
 * browser that has no session.
 */
function PriceSentinelPage() {
  const { token } = Route.useSearch()

  return (
    <PublicLayout showMainContainer>
      <div className='mx-auto w-full max-w-5xl px-4 py-8'>
        <PriceSentinelEditor token={token} />
      </div>
    </PublicLayout>
  )
}
