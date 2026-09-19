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
import type { PricingData } from '@/features/pricing/types'

import type {
  CoverageRow,
  LandingModelsConfig,
  RouteGroup,
  RouteKind,
} from './default-landing-models'

/** Caps so one large group cannot push everything else off the page. */
const MAX_ROUTES = 12
const MAX_MODELS_PER_ROUTE = 6
const MAX_COVERAGE_ROWS = 12
const MAX_ROUTES_PER_ROW = 4

export type DeriveOptions = {
  /** Used for a group the operator has not described in the group picker. */
  neutralRouteDescription: string
  /** Heading for a model family, given how many models it contributes. */
  familyDescription: (modelCount: number) => string
}

/**
 * `usable_group` carries a plain description string in the payload the server
 * actually sends, while the client type declares an object. Read both rather
 * than trust either.
 */
function readGroupDescription(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (value && typeof value === 'object') {
    const desc = (value as { desc?: unknown }).desc
    if (typeof desc === 'string') return desc.trim()
  }
  return ''
}

/**
 * The kinds are styling buckets, not facts the payload carries. `free` is the
 * one that is legible from the name; everything else is presented as a priority
 * route, because claiming a route is cheap without knowing its pricing would be
 * inventing the very thing this derivation exists to stop inventing.
 */
function routeKind(group: string): RouteKind {
  return /free/i.test(group) ? 'free' : 'priority'
}

/**
 * Builds the landing page's route and coverage data from what the site actually
 * serves, so the console stops advertising groups and model names that exist
 * only in the shipped defaults.
 *
 * Returns `null` when the payload carries no models at all, which is the caller's
 * cue to keep whatever it had.
 */
export function deriveLandingModels(
  pricing: PricingData | null | undefined,
  options: DeriveOptions
): LandingModelsConfig | null {
  const models = Array.isArray(pricing?.data) ? pricing.data : []
  if (models.length === 0) return null

  const groups = new Map<string, string[]>()
  const vendors = new Map<number, { count: number; groups: Set<string> }>()

  for (const model of models) {
    const name = model?.model_name
    if (typeof name !== 'string' || name === '') continue

    // `enable_groups` is what the group picker offers this model under.
    const servedIn = (model.enable_groups ?? []).filter(
      (group): group is string => typeof group === 'string' && group !== ''
    )
    for (const group of servedIn) {
      const bucket = groups.get(group)
      if (bucket) bucket.push(name)
      else groups.set(group, [name])
    }

    const vendorId = model.vendor_id
    if (typeof vendorId === 'number') {
      const entry = vendors.get(vendorId)
      if (entry) {
        entry.count += 1
        for (const group of servedIn) entry.groups.add(group)
      } else {
        vendors.set(vendorId, { count: 1, groups: new Set(servedIn) })
      }
    }
  }

  if (groups.size === 0) return null

  const usableGroup = (pricing?.usable_group ?? {}) as Record<string, unknown>
  const serviceRoutes: RouteGroup[] = [...groups.entries()]
    // Widest groups first: they are the ones worth showing.
    .sort(
      ([leftGroup, left], [rightGroup, right]) =>
        right.length - left.length || leftGroup.localeCompare(rightGroup)
    )
    .slice(0, MAX_ROUTES)
    .map(([group, names]) => ({
      id: group,
      kind: routeKind(group),
      group,
      description:
        readGroupDescription(usableGroup[group]) ||
        options.neutralRouteDescription,
      models: names.slice(0, MAX_MODELS_PER_ROUTE),
    }))

  const vendorNames = new Map<number, string>()
  for (const vendor of pricing?.vendors ?? []) {
    if (typeof vendor?.id === 'number' && typeof vendor.name === 'string') {
      if (vendor.name !== '') vendorNames.set(vendor.id, vendor.name)
    }
  }

  const coverage: CoverageRow[] = [...vendors.entries()]
    .filter(([vendorId]) => vendorNames.has(vendorId))
    .sort(
      ([leftId, left], [rightId, right]) =>
        right.count - left.count ||
        (vendorNames.get(leftId) ?? '').localeCompare(
          vendorNames.get(rightId) ?? ''
        )
    )
    .slice(0, MAX_COVERAGE_ROWS)
    .map(([vendorId, entry]) => {
      const name = vendorNames.get(vendorId) ?? ''
      return {
        mark: name,
        family: name,
        description: options.familyDescription(entry.count),
        routeType: [...entry.groups]
          .sort((left, right) => left.localeCompare(right))
          .slice(0, MAX_ROUTES_PER_ROW)
          .join(' · '),
      }
    })

  return { serviceRoutes, coverage }
}

/**
 * Combines the operator's override with the derived data: whatever the override
 * names is used as written, and anything it does not cover still appears.
 */
export function mergeLandingModels(
  override: LandingModelsConfig | null,
  derived: LandingModelsConfig | null
): LandingModelsConfig | null {
  if (!override && !derived) return null
  if (!override) return derived
  if (!derived) return override

  const overriddenGroups = new Set(
    override.serviceRoutes.map((route) => route.group)
  )
  const overriddenFamilies = new Set(override.coverage.map((row) => row.family))

  return {
    serviceRoutes: [
      ...override.serviceRoutes,
      ...derived.serviceRoutes.filter(
        (route) => !overriddenGroups.has(route.group)
      ),
    ],
    coverage: [
      ...override.coverage,
      ...derived.coverage.filter((row) => !overriddenFamilies.has(row.family)),
    ],
  }
}
