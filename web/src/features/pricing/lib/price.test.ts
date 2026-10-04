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
    expect(formatGroupPrice(priced, 'default', 'input', 'M', false, 1, 1, {
      default: 1,
    })).toBe('-')
  })
})
