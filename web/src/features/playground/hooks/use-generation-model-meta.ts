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
import { useMemo } from 'react'

import { getPricing } from '@/features/pricing/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { GenerationModelMeta } from '../components/generation/generation-utils'

/**
 * Model name → what the pricing payload says the model can do.
 *
 * The playground's own model list is every model the account can reach, with no
 * capability attached, so a picker cannot tell an image model from a text one.
 * Pricing already publishes `supported_endpoint_types` and `output_modalities`
 * per model, so the generation tabs read it from there rather than adding a
 * second endpoint. It shares the `pricing` query key with the pricing page, so
 * the two never fetch it twice.
 *
 * A failed fetch resolves to an empty map, which reads as "nothing known" and
 * leaves the pickers unfiltered — the same as before this filter existed.
 */
export function useGenerationModelMeta(): Map<string, GenerationModelMeta> {
  const { data } = useQuery({
    queryKey: ['pricing'],
    queryFn: async () => requireServerSuccess(await getPricing()),
    staleTime: 5 * 60 * 1000,
  })

  return useMemo(() => {
    const meta = new Map<string, GenerationModelMeta>()
    for (const model of data?.data ?? []) {
      meta.set(model.model_name, {
        endpoints: model.supported_endpoint_types ?? [],
        outputs: model.output_modalities ?? [],
      })
    }
    return meta
  }, [data])
}
