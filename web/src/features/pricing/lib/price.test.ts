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

import type { PricingModel } from '../types'
import {
  formatFixedPrice,
  formatGroupPrice,
  formatPrice,
  formatRequestPrice,
  resolveGroupTokenPrices,
} from './price'

function model(overrides: Partial<PricingModel> = {}): PricingModel {
  return {
    model_name: 'example-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 3,
    enable_groups: ['default'],
    group_ratio: { default: 1 },
    ...overrides,
  } as PricingModel
}

// Free and unpriced are different states. A model configured at zero is served
// and costs nothing; one with no price at all cannot be served, and rendering
// both as "-" told a visitor that a free model was missing.
describe('a zero price is free, not unpriced', () => {
  it('shows a per-request model priced at zero as free', () => {
    const free = model({ quota_type: 1, model_price: 0 })
    expect(formatRequestPrice(free)).toBe('Free')
    expect(formatFixedPrice(free, 'default', false, 1, 1, { default: 1 })).toBe(
      'Free'
    )
  })

  it('shows an unpriced per-request model as having no price', () => {
    const unpriced = model({ quota_type: 1 })
    expect(formatRequestPrice(unpriced)).toBe('-')
    expect(
      formatFixedPrice(unpriced, 'default', false, 1, 1, { default: 1 })
    ).toBe('-')
  })

  it('keeps a real per-request price readable', () => {
    const priced = model({ quota_type: 1, model_price: 0.02 })
    expect(formatRequestPrice(priced)).toBe('$0.02')
  })

  // A free model has no token price, but reporting "-" next to its free request
  // price read as a second, missing number rather than the absence of one.
  it('leaves the token columns to the free model’s request price', () => {
    const free = model({ quota_type: 1, model_price: 0 })
    expect(formatPrice(free, 'input', 'M')).not.toBe('-')
  })

  it('still hides the token columns of a paid per-request model', () => {
    const priced = model({ quota_type: 1, model_price: 0.02 })
    expect(formatPrice(priced, 'input', 'M')).toBe('-')
    expect(
      formatGroupPrice(priced, 'default', 'input', 'M', false, 1, 1, {
        default: 1,
      })
    ).toBe('-')
  })
})

// A group can be repriced through its own ratio. Rendering the model's global
// ratio then advertises a price nobody in that group is charged, which is how
// a group billed at 1.01/4.01 was shown as 0.011/0.045.
describe('a group bills the ratio it overrides, not the global one', () => {
  const repriced = model({
    model_ratio: 0.1875,
    completion_ratio: 4,
    cache_ratio: 0.02,
    group_ratio: { default: 1, bare: 0.06 },
  })
  const overrides = {
    ratio: { bare: { 'example-model': 8.4166666667 } },
    completion: { bare: { 'example-model': 3.9702970297 } },
    cache: { bare: { 'example-model': 0.0297029703 } },
  }

  it('resolves the overridden per-million prices', () => {
    const prices = resolveGroupTokenPrices(
      repriced,
      'bare',
      { bare: { 'example-model': 'ratio' } },
      0.06,
      overrides
    )
    expect(prices).not.toBeNull()
    expect(prices?.input).toBeCloseTo(1.01, 6)
    expect(prices?.output).toBeCloseTo(4.01, 6)
    expect(prices?.cache).toBeCloseTo(0.03, 6)
  })

  it('leaves expression and per-request groups to their own rendering', () => {
    expect(
      resolveGroupTokenPrices(
        repriced,
        'bare',
        { bare: { 'example-model': 'tiered_expr' } },
        0.06,
        overrides
      )
    ).toBeNull()
    expect(
      resolveGroupTokenPrices(
        repriced,
        'bare',
        { bare: { 'example-model': 'price' } },
        0.06,
        overrides
      )
    ).toBeNull()
  })

  it('leaves a group without an override to the global price', () => {
    expect(
      resolveGroupTokenPrices(repriced, 'default', undefined, 1, overrides)
    ).toBeNull()
  })

  it('shows the overridden price in the group price column', () => {
    expect(
      formatGroupPrice(
        repriced,
        'bare',
        'input',
        'M',
        false,
        1,
        1,
        {
          default: 1,
          bare: 0.06,
        },
        overrides
      )
    ).toBe('$1.01')
    expect(
      formatGroupPrice(
        repriced,
        'bare',
        'output',
        'M',
        false,
        1,
        1,
        {
          default: 1,
          bare: 0.06,
        },
        overrides
      )
    ).toBe('$4.01')
  })
})

// A group may also carry its own per-call price, which the fixed-price column
// used to ignore in favour of the model's global figure.
describe('a per-call override replaces the global per-call price', () => {
  it('charges the group its own per-call price', () => {
    const priced = model({ quota_type: 1, model_price: 0.02 })
    const overrides = { price: { bare: { 'example-model': 0.05 } } }
    expect(
      formatFixedPrice(
        priced,
        'bare',
        false,
        1,
        1,
        { bare: 0.5 },
        undefined,
        overrides
      )
    ).toBe('$0.025')
  })
})
