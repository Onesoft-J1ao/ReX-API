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

import type { PricingModel } from '../../types'
import {
  getGroupPricingForModel,
  hasGroupPricingOverride,
  listGroupPricing,
} from '../group-pricing'

function model(overrides: Partial<PricingModel> = {}): PricingModel {
  return {
    model_name: 'kimi-k3',
    quota_type: 0,
    enable_groups: ['default', '国模高速'],
    group_ratio: { default: 1, 国模高速: 0.12 },
    ...overrides,
  } as PricingModel
}

describe('getGroupPricingForModel', () => {
  // The whole point: a per-token model priced per call in one group. Without
  // this the square multiplies one global price by the group ratio and shows
  // the same figure in both groups.
  it('lets a group override switch a token-priced model to per call', () => {
    const pricing = getGroupPricingForModel(
      model(),
      '国模高速',
      { 国模高速: { 'kimi-k3': 'price' } },
      0.12
    )
    expect(pricing).toEqual({
      group: '国模高速',
      source: 'price',
      overridden: true,
      groupRatio: 0.12,
    })
  })

  it('falls back to the global mode when the group has no override', () => {
    const pricing = getGroupPricingForModel(
      model(),
      'default',
      { 国模高速: { 'kimi-k3': 'price' } },
      1
    )
    expect(pricing.source).toBe('ratio')
    expect(pricing.overridden).toBe(false)
  })

  it('reads the global mode from quota_type for a per-request model', () => {
    const pricing = getGroupPricingForModel(
      model({ quota_type: 1 }),
      'default',
      undefined,
      1
    )
    expect(pricing.source).toBe('price')
  })

  // Publishing the mode an operator chose but the gateway does not recognise
  // would show a price nothing is charged at.
  it('ignores an unrecognised source', () => {
    const pricing = getGroupPricingForModel(
      model(),
      'default',
      { default: { 'kimi-k3': 'per-kg' } },
      1
    )
    expect(pricing.source).toBe('ratio')
    expect(pricing.overridden).toBe(false)
  })

  // An override that restates the global mode is not a difference, so the card
  // must not announce it as one.
  it('does not report a restated global mode as an override', () => {
    const pricing = getGroupPricingForModel(
      model(),
      'default',
      { default: { 'kimi-k3': 'ratio' } },
      1
    )
    expect(pricing.overridden).toBe(false)
  })

  it('handles a missing group_billing_source entirely', () => {
    expect(
      getGroupPricingForModel(model(), 'default', undefined, 1).source
    ).toBe('ratio')
  })
})

describe('listGroupPricing', () => {
  it('keeps the declared group order and each group ratio', () => {
    const result = listGroupPricing(
      model(),
      ['default', '国模高速'],
      { 国模高速: { 'kimi-k3': 'price' } },
      { default: 1, 国模高速: 0.12 }
    )
    expect(result.map((pricing) => pricing.group)).toEqual([
      'default',
      '国模高速',
    ])
    expect(result[1].source).toBe('price')
    expect(result[1].groupRatio).toBe(0.12)
  })

  // A group with no ratio configured bills at 1, not at NaN or zero.
  it('defaults a missing ratio to 1', () => {
    const result = listGroupPricing(model(), ['new-group'], undefined, {})
    expect(result[0].groupRatio).toBe(1)
  })
})

describe('hasGroupPricingOverride', () => {
  it('is true when any group differs', () => {
    expect(
      hasGroupPricingOverride(model(), ['default', '国模高速'], {
        国模高速: { 'kimi-k3': 'price' },
      })
    ).toBe(true)
  })

  it('is false when no group differs', () => {
    expect(
      hasGroupPricingOverride(model(), ['default', '国模高速'], undefined)
    ).toBe(false)
  })
})