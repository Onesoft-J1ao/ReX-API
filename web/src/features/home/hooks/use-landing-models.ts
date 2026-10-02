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
import i18next from 'i18next'
import { useEffect, useState } from 'react'

import { getPricing } from '@/features/pricing/api'

import { getLandingModels } from '../api'
import type { LandingModelsConfig } from '../lib/default-landing-models'
import {
  deriveLandingModels,
  mergeLandingModels,
} from '../lib/derive-landing-models'

const STORAGE_KEY = 'landing_models'

/** Rendered only until the first response lands; the sections are list-driven. */
const EMPTY_CONFIG: LandingModelsConfig = { serviceRoutes: [], coverage: [] }

function isConfig(value: unknown): value is LandingModelsConfig {
  const config = value as Partial<LandingModelsConfig> | null
  return Boolean(
    config &&
      Array.isArray(config.serviceRoutes) &&
      Array.isArray(config.coverage)
  )
}

/** The operator's override. Returns `null` when unset, so the live data is used. */
function parseOverride(raw: string | undefined | null): LandingModelsConfig | null {
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as unknown
    return isConfig(parsed) ? parsed : null
  } catch {
    return null
  }
}

/**
 * The landing page's route and coverage data.
 *
 * Sourced from the site itself: the operator's `LandingModels` override is used
 * as written and the rest is derived from the groups and models the pricing
 * endpoint actually serves. The shipped defaults that used to back this are gone
 * on purpose -- a page that lists groups the site does not run, or model names it
 * does not serve, is worse than one that lists fewer.
 */
export function useLandingModels(): LandingModelsConfig {
  const [config, setConfig] = useState<LandingModelsConfig>(() => {
    if (typeof window === 'undefined') return EMPTY_CONFIG
    const cached = window.localStorage.getItem(STORAGE_KEY)
    if (!cached) return EMPTY_CONFIG
    try {
      const parsed = JSON.parse(cached) as unknown
      return isConfig(parsed) ? parsed : EMPTY_CONFIG
    } catch {
      return EMPTY_CONFIG
    }
  })

  useEffect(() => {
    let mounted = true

    const resolve = async () => {
      const [overrideResult, pricingResult] = await Promise.allSettled([
        getLandingModels(),
        getPricing(),
      ])
      if (!mounted) return

      const override =
        overrideResult.status === 'fulfilled' && overrideResult.value?.success
          ? parseOverride(overrideResult.value.data)
          : null

      const derived =
        pricingResult.status === 'fulfilled'
          ? deriveLandingModels(pricingResult.value, {
              neutralRouteDescription: i18next.t(
                'Models this site serves under one group.'
              ),
              familyDescription: (modelCount) =>
                i18next.t('{{count}} models', { count: modelCount }),
            })
          : null

      const merged = mergeLandingModels(override, derived)
      if (!merged) return

      setConfig(merged)
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(merged))
      } catch {
        // Storage unavailable; the config still applies for this page view.
      }
    }

    void resolve()
    return () => {
      mounted = false
    }
  }, [])

  return config
}
