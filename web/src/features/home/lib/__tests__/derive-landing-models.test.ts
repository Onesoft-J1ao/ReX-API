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
import { describe, expect, it } from 'vitest'

import type { PricingData, PricingModel } from '@/features/pricing/types'

import type { LandingModelsConfig } from '../default-landing-models'
import {
  deriveLandingModels,
  mergeLandingModels,
} from '../derive-landing-models'

const OPTIONS = {
  neutralRouteDescription: 'NEUTRAL',
  familyDescription: (count: number) => `${count} models`,
}

/** Only the fields the derivation reads; the rest are filler. */
function model(input: {
  model_name: string
  vendor_id?: number
  enable_groups?: string[]
}): PricingModel {
  return {
    id: 0,
    model_name: input.model_name,
    vendor_id: input.vendor_id,
    enable_groups: input.enable_groups ?? [],
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
  }
}

function pricing(overrides: Partial<PricingData>): PricingData {
  return {
    success: true,
    data: [],
    vendors: [],
    group_ratio: {},
    usable_group: {},
    supported_endpoint: {},
    auto_groups: [],
    ...overrides,
  }
}

describe('deriveLandingModels', () => {
  it('lists the groups the site actually serves, widest first', () => {
    const derived = deriveLandingModels(
      pricing({
        data: [
          model({ model_name: 'a', vendor_id: 1, enable_groups: ['small'] }),
          model({ model_name: 'b', vendor_id: 1, enable_groups: ['wide'] }),
          model({ model_name: 'c', vendor_id: 1, enable_groups: ['wide'] }),
        ],
      }),
      OPTIONS
    )

    expect(derived?.serviceRoutes.map((route) => route.group)).toEqual([
      'wide',
      'small',
    ])
    expect(derived?.serviceRoutes[0].models).toEqual(['b', 'c'])
  })

  it('never repeats the operator group notes on the public page', () => {
    // Those notes name the upstream pool serving a group ("upstream free pool",
    // "third-party pool"), which members need and visitors should not see.
    const derived = deriveLandingModels(
      pricing({
        data: [
          model({ model_name: 'a', vendor_id: 1, enable_groups: ['described'] }),
          model({ model_name: 'b', vendor_id: 1, enable_groups: ['bare'] }),
        ],
        usable_group: {
          described: '🟢upstream free pool',
          bare: 'third-party pool, refilled every 15m',
        } as unknown as PricingData['usable_group'],
      }),
      OPTIONS
    )

    expect(derived?.serviceRoutes.map((route) => route.description)).toEqual([
      'NEUTRAL',
      'NEUTRAL',
    ])
  })

  it('marks a free group free and leaves the rest as priority', () => {
    const derived = deriveLandingModels(
      pricing({
        data: [
          model({ model_name: 'a', vendor_id: 1, enable_groups: ['free'] }),
          model({ model_name: 'b', vendor_id: 1, enable_groups: ['paid'] }),
        ],
      }),
      OPTIONS
    )

    const kinds = Object.fromEntries(
      (derived?.serviceRoutes ?? []).map((route) => [route.group, route.kind])
    )
    expect(kinds.free).toBe('free')
    expect(kinds.paid).toBe('priority')
  })

  it('builds coverage rows from the vendors that serve models', () => {
    const derived = deriveLandingModels(
      pricing({
        data: [
          model({ model_name: 'a', vendor_id: 7, enable_groups: ['g1'] }),
          model({ model_name: 'b', vendor_id: 7, enable_groups: ['g2'] }),
          model({ model_name: 'c', vendor_id: 9, enable_groups: ['g1'] }),
        ],
        vendors: [
          { id: 7, name: 'Anthropic' },
          { id: 9, name: 'Google' },
        ],
      }),
      OPTIONS
    )

    expect(derived?.coverage).toEqual([
      {
        mark: 'Anthropic',
        family: 'Anthropic',
        description: '2 models',
        routeType: 'g1 · g2',
      },
      {
        mark: 'Google',
        family: 'Google',
        description: '1 models',
        routeType: 'g1',
      },
    ])
  })

  it('drops a vendor the payload does not name', () => {
    const derived = deriveLandingModels(
      pricing({
        data: [model({ model_name: 'a', vendor_id: 7, enable_groups: ['g'] })],
        vendors: [],
      }),
      OPTIONS
    )

    expect(derived?.coverage).toEqual([])
  })

  it('returns nothing when the payload carries no models', () => {
    expect(deriveLandingModels(pricing({}), OPTIONS)).toBeNull()
    expect(deriveLandingModels(null, OPTIONS)).toBeNull()
    expect(deriveLandingModels(undefined, OPTIONS)).toBeNull()
  })

  it('caps what a single group can show', () => {
    const names = Array.from({ length: 20 }, (_, index) => `m${index}`)
    const derived = deriveLandingModels(
      pricing({
        data: names.map((model_name) =>
          model({ model_name, vendor_id: 1, enable_groups: ['g'] })
        ),
        vendors: [{ id: 1, name: 'V' }],
      }),
      OPTIONS
    )

    expect(derived?.serviceRoutes[0].models).toHaveLength(6)
  })
})

describe('mergeLandingModels', () => {
  const derived: LandingModelsConfig = {
    serviceRoutes: [
      {
        id: 'a',
        kind: 'priority',
        group: 'a',
        description: 'derived a',
        models: ['m1'],
      },
      {
        id: 'b',
        kind: 'priority',
        group: 'b',
        description: 'derived b',
        models: ['m2'],
      },
    ],
    coverage: [{ mark: 'A', family: 'A', description: '', routeType: '' }],
  }

  it('keeps the override as written and adds what it does not cover', () => {
    const override: LandingModelsConfig = {
      serviceRoutes: [
        {
          id: 'a',
          kind: 'value',
          group: 'a',
          description: 'hand written',
          models: ['chosen'],
        },
      ],
      coverage: [],
    }

    const merged = mergeLandingModels(override, derived)

    expect(merged?.serviceRoutes.map((route) => route.group)).toEqual(['a', 'b'])
    expect(merged?.serviceRoutes[0].description).toBe('hand written')
    expect(merged?.serviceRoutes[0].kind).toBe('value')
    expect(merged?.coverage).toHaveLength(1)
  })

  it('passes through whichever side exists', () => {
    expect(mergeLandingModels(null, derived)).toEqual(derived)
    expect(mergeLandingModels(derived, null)).toEqual(derived)
    expect(mergeLandingModels(null, null)).toBeNull()
  })
})
