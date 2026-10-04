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
import type { PricingModel } from '../types'

/**
 * How a model is billed in one group.
 *
 * `ratio` and `tiered_expr` are both token-priced and differ in how the price
 * is derived, so the square treats them alike for display. `price` is the odd
 * one out: it is a fixed amount per request, which no token price can express.
 */
export type GroupBillingSource = 'ratio' | 'price' | 'tiered_expr'

export type GroupPricing = {
  group: string
  /** The source the gateway will actually bill this model by in this group. */
  source: GroupBillingSource
  /** True when the group's override differs from the model's global mode. */
  overridden: boolean
  groupRatio: number
}

const KNOWN_SOURCES = new Set<string>(['ratio', 'price', 'tiered_expr'])

function asSource(value: string | undefined): GroupBillingSource | null {
  if (!value || !KNOWN_SOURCES.has(value)) return null
  return value as GroupBillingSource
}

/**
 * The pricing source the gateway would use for a model in a group.
 *
 * A group override wins over the model's global billing mode. Without this the
 * square can only multiply one global price by a group ratio, so a model billed
 * per call in one group and per token in another shows the same figure in both
 * — and a per-call price that is not the global one is simply invisible.
 *
 * An absent `group_billing_source` means the gateway publishes no overrides
 * (this is the common case), and every group falls back to the global mode.
 */
export function getGroupPricingForModel(
  model: PricingModel,
  group: string,
  groupBillingSource: Record<string, Record<string, string>> | undefined,
  groupRatio: number
): GroupPricing {
  const override = asSource(groupBillingSource?.[group]?.[model.model_name])
  const global = model.quota_type === 1 ? 'price' : 'ratio'
  const source: GroupBillingSource = override ?? global
  return {
    group,
    source,
    overridden: override !== null && override !== global,
    groupRatio,
  }
}

/**
 * Every group this model is reachable in, with the pricing that applies there.
 *
 * Groups arrive in the order the model declares them so the detail table keeps
 * a stable order between loads.
 */
export function listGroupPricing(
  model: PricingModel,
  groups: readonly string[],
  groupBillingSource: Record<string, Record<string, string>> | undefined,
  groupRatioByGroup: Record<string, number>
): GroupPricing[] {
  return groups.map((group) =>
    getGroupPricingForModel(
      model,
      group,
      groupBillingSource,
      typeof groupRatioByGroup[group] === 'number' &&
        Number.isFinite(groupRatioByGroup[group])
        ? groupRatioByGroup[group]
        : 1
    )
  )
}

/**
 * Whether any group prices this model differently from the model's own mode.
 *
 * The card uses it to say so once, rather than marking every group row.
 */
export function hasGroupPricingOverride(
  model: PricingModel,
  groups: readonly string[],
  groupBillingSource: Record<string, Record<string, string>> | undefined
): boolean {
  return listGroupPricing(model, groups, groupBillingSource, {}).some(
    (pricing) => pricing.overridden
  )
}