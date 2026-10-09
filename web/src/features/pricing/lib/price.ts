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
import { t } from 'i18next'

import { formatBillingCurrencyFromUSD } from '@/lib/currency'

import { QUOTA_TYPE_VALUES, TOKEN_UNIT_DIVISORS } from '../constants'
import type { PricingModel, TokenUnit, PriceType } from '../types'
import { getGroupPricingForModel } from './group-pricing'
import { getConfiguredGroupRatio, getDisplayGroupRatio } from './model-helpers'

export const FREE_REQUEST_PRICE_THRESHOLD_USD = 0.000002

// ----------------------------------------------------------------------------
// Price Calculation Utilities
// ----------------------------------------------------------------------------

/**
 * Strip trailing zeros from formatted price string while preserving currency symbols
 */
export function stripTrailingZeros(formatted: string): string {
  // Match currency symbol at start, number, and potential 'k' suffix
  const match = formatted.match(/^([^\d-]*)([-\d,]+\.?\d*)(k?)$/)
  if (!match) return formatted

  const [, symbol, number, suffix] = match

  // Remove commas for processing
  const cleanNumber = number.replaceAll(',', '')

  // Convert to number and back to remove trailing zeros
  const parsed = Number.parseFloat(cleanNumber)
  if (Number.isNaN(parsed)) return formatted

  // Convert to string, which automatically removes trailing zeros
  let result = parsed.toString()

  // If the result is in scientific notation, format it properly
  if (result.includes('e')) {
    result = parsed.toFixed(20).replace(/\.?0+$/, '')
  }

  return `${symbol}${result}${suffix}`
}

/**
 * Resolve the model ratio a group actually bills by.
 *
 * A model can be fed by a different upstream per group, so the gateway lets a
 * group override the per-token ratio. When it does, that figure replaces the
 * model's global ratio before any multiplier applies; without this the square
 * would show every group the same price and hide the override entirely.
 */
export function resolveModelRatio(
  model: PricingModel,
  group: string | undefined,
  overrides: GroupPricingOverrides
): number | undefined {
  return withGroupPricing(model, group, overrides).model_ratio ?? undefined
}

/**
 * Apply a group's per-model overrides to a copy of the model.
 *
 * A model fed by a different upstream per group can differ in all three
 * figures the square prices from (input ratio, output multiplier, cache
 * multiplier). Applying them here keeps the four format* helpers from each
 * having to know which maps exist.
 */
function withGroupPricing(
  model: PricingModel,
  group: string | undefined,
  overrides: GroupPricingOverrides
): PricingModel {
  if (!group) return model
  const ratio = overrides.ratio?.[group]?.[model.model_name]
  const completion = overrides.completion?.[group]?.[model.model_name]
  const cache = overrides.cache?.[group]?.[model.model_name]
  return {
    ...model,
    model_ratio:
      typeof ratio === 'number' && Number.isFinite(ratio)
        ? ratio
        : (model.model_ratio ?? undefined),
    completion_ratio:
      typeof completion === 'number' && Number.isFinite(completion)
        ? completion
        : model.completion_ratio,
    cache_ratio:
      typeof cache === 'number' && Number.isFinite(cache)
        ? cache
        : model.cache_ratio,
  }
}

/** The three per-group per-model override maps the gateway publishes. */
export type GroupPricingOverrides = {
  ratio?: Record<string, Record<string, number>>
  completion?: Record<string, Record<string, number>>
  cache?: Record<string, Record<string, number>>
}

/**
 * Calculate token price in USD.
 *
 * Returns NaN when the required ratio field is missing/null so callers can
 * skip rendering that price type.
 */
function calculateTokenPrice(
  model: PricingModel,
  type: PriceType,
  ratio: number
): number {
  // No configured ratio means no price, which is not a zero price: an
  // unpriced model must not render as free. NaN makes the callers below
  // skip the row, which is what this function documents.
  const base = (model.model_ratio ?? Number.NaN) * 2 * ratio

  switch (type) {
    case 'input':
      return base
    case 'output':
      return base * model.completion_ratio
    case 'cache':
      return hasRatio(model.cache_ratio)
        ? base * Number(model.cache_ratio)
        : Number.NaN
    case 'create_cache':
      return hasRatio(model.create_cache_ratio)
        ? base * Number(model.create_cache_ratio)
        : Number.NaN
    case 'image':
      return hasRatio(model.image_ratio)
        ? base * Number(model.image_ratio)
        : Number.NaN
    case 'audio_input':
      return hasRatio(model.audio_ratio)
        ? base * Number(model.audio_ratio)
        : Number.NaN
    case 'audio_output':
      return hasRatio(model.audio_ratio) &&
        hasRatio(model.audio_completion_ratio)
        ? base *
            Number(model.audio_ratio) *
            Number(model.audio_completion_ratio)
        : Number.NaN
  }
}

function hasRatio(value: number | null | undefined): boolean {
  return value !== undefined && value !== null && Number.isFinite(Number(value))
}

/**
 * Apply recharge rate to price
 *
 * priceRate represents how much users need to recharge (in the display currency)
 * to get 1 USD credit. usdExchangeRate is the real exchange rate.
 *
 * The returned value will be formatted by formatCurrencyFromUSD, which will
 * multiply by the display currency's exchange rate.
 *
 * Examples:
 *
 * 1. Display currency = USD:
 *    - Model: 1 USD
 *    - priceRate = 0.5 (recharge $0.5 to get $1 credit)
 *    - usdExchangeRate = 1
 *    - Return: 1 × 0.5 / 1 = 0.5
 *    - formatCurrencyFromUSD(0.5) → $0.5 ✓
 *
 * 2. Display currency = CNY:
 *    - Model: 1 USD
 *    - priceRate = 4 (recharge ¥4 to get $1 credit)
 *    - usdExchangeRate = 7 (real rate: 1 USD = ¥7)
 *    - Return: 1 × 4 / 7 = 0.571
 *    - formatCurrencyFromUSD(0.571) → 0.571 × 7 = ¥4 ✓
 *    - Normal price: ¥7, Recharge price: ¥4 (cheaper!)
 */
function applyRechargeRate(
  price: number,
  showWithRecharge: boolean,
  priceRate: number,
  usdExchangeRate: number
): number {
  if (!showWithRecharge) return price
  return (price * priceRate) / usdExchangeRate
}

/**
 * Format token-based price for display
 */
export function formatPrice(
  model: PricingModel,
  type: PriceType,
  tokenUnit: TokenUnit,
  showWithRecharge = false,
  priceRate = 1,
  usdExchangeRate = 1,
  selectedGroup?: string,
  showCurrencySymbol = true,
  overrides: GroupPricingOverrides = {}
): string {
  // A per-request model has no token price to show; its price belongs on the
  // request column. A free one still has a price of zero, so leave the label to
  // that column rather than claiming there is none.
  if (model.quota_type === QUOTA_TYPE_VALUES.REQUEST && model.model_price) {
    return '-'
  }

  const displayGroupRatio = getDisplayGroupRatio(model, selectedGroup)

  const priced = withGroupPricing(model, selectedGroup, overrides)
  let priceInUSD = calculateTokenPrice(priced, type, displayGroupRatio)
  // A token model is free when either its input or output price for the
  // displayed million-token unit is below the public free threshold. Keep the
  // label consistent across both columns/cards when one side is effectively
  // free.
  if (isTokenPriceFree(priced, displayGroupRatio)) {
    return t('Free')
  }
  priceInUSD = applyRechargeRate(
    priceInUSD,
    showWithRecharge,
    priceRate,
    usdExchangeRate
  )

  const price = priceInUSD / TOKEN_UNIT_DIVISORS[tokenUnit]
  return formatBillingCurrencyFromUSD(price, {
    showSymbol: showCurrencySymbol,
    digitsLarge: 4,
    digitsSmall: 6,
    abbreviate: false,
  })
}

/**
 * Format price for a specific group (token-based)
 */
export function formatGroupPrice(
  model: PricingModel,
  group: string,
  type: PriceType,
  tokenUnit: TokenUnit,
  showWithRecharge = false,
  priceRate = 1,
  usdExchangeRate = 1,
  groupRatio: Record<string, number>,
  overrides: GroupPricingOverrides = {}
): string {
  // Same rule as formatPrice: a per-request model shows nothing on a token
  // column, but a free one has a real price of zero.
  if (model.quota_type === QUOTA_TYPE_VALUES.REQUEST && model.model_price) {
    return '-'
  }

  const ratio = getConfiguredGroupRatio(groupRatio, group)
  const priced = withGroupPricing(model, group, overrides)
  let priceInUSD = calculateTokenPrice(priced, type, ratio)
  if (isTokenPriceFree(priced, ratio)) {
    return t('Free')
  }
  priceInUSD = applyRechargeRate(
    priceInUSD,
    showWithRecharge,
    priceRate,
    usdExchangeRate
  )

  const price = priceInUSD / TOKEN_UNIT_DIVISORS[tokenUnit]
  return formatBillingCurrencyFromUSD(price, {
    digitsLarge: 4,
    digitsSmall: 6,
    abbreviate: false,
  })
}

/** Whether a token model should be presented as free for the selected group. */
export function isTokenPriceFree(model: PricingModel, ratio: number): boolean {
  if (model.quota_type === QUOTA_TYPE_VALUES.REQUEST) return false
  const input = calculateTokenPrice(model, 'input', ratio)
  const output = calculateTokenPrice(model, 'output', ratio)
  return [input, output].some(
    (price) =>
      Number.isFinite(price) && price < FREE_REQUEST_PRICE_THRESHOLD_USD
  )
}

/**
 * Format fixed price for pay-per-request models (with specific group)
 */
export function formatFixedPrice(
  model: PricingModel,
  group: string,
  showWithRecharge = false,
  priceRate = 1,
  usdExchangeRate = 1,
  groupRatio: Record<string, number>,
  groupBillingSource?: Record<string, Record<string, string>>
): string {
  // A group may pin a token-priced model to per-request billing, and then this
  // group is charged the model's per-call price.
  const perCallSource =
    model.quota_type === QUOTA_TYPE_VALUES.REQUEST ||
    getGroupPricingForModel(model, group, groupBillingSource, 1).source ===
      'price'
  if (!perCallSource || model.model_price == null) {
    // A model with no per-call price at all is unpriced, which is not the same
    // as free. One configured at zero is free and falls through below.
    return '-'
  }

  const ratio = getConfiguredGroupRatio(groupRatio, group)
  let priceInUSD = model.model_price * ratio
  if (priceInUSD < FREE_REQUEST_PRICE_THRESHOLD_USD) return t('Free')

  priceInUSD = applyRechargeRate(
    priceInUSD,
    showWithRecharge,
    priceRate,
    usdExchangeRate
  )

  return formatBillingCurrencyFromUSD(priceInUSD, {
    digitsLarge: 4,
    digitsSmall: 6,
    abbreviate: false,
  })
}

/**
 * Format fixed price for pay-per-request models (minimum price from all groups)
 *
 * A group may pin a token-priced model to per-request billing. The gateway
 * then charges that group's requests the model's per-call price, so the same
 * figure has to be shown here; refusing on quota_type alone rendered a dash for
 * a price the viewer is really charged.
 */
export function formatRequestPrice(
  model: PricingModel,
  showWithRecharge = false,
  priceRate = 1,
  usdExchangeRate = 1,
  selectedGroup?: string,
  showCurrencySymbol = true,
  groupBillingSource?: Record<string, Record<string, string>>
): string {
  const perCallSource =
    model.quota_type === QUOTA_TYPE_VALUES.REQUEST ||
    getGroupPricingForModel(model, selectedGroup ?? '', groupBillingSource, 1)
      .source === 'price'
  // A pinned model with no per-call price configured has nothing to charge per
  // request; the gateway rejects that pin rather than billing zero. A model
  // configured at zero is free, not unpriced, and falls through below.
  if (!perCallSource || model.model_price == null) {
    return '-'
  }

  const displayGroupRatio = getDisplayGroupRatio(model, selectedGroup)

  let priceInUSD = model.model_price * displayGroupRatio
  if (priceInUSD < FREE_REQUEST_PRICE_THRESHOLD_USD) {
    return t('Free')
  }

  priceInUSD = applyRechargeRate(
    priceInUSD,
    showWithRecharge,
    priceRate,
    usdExchangeRate
  )

  return formatBillingCurrencyFromUSD(priceInUSD, {
    showSymbol: showCurrencySymbol,
    digitsLarge: 4,
    digitsSmall: 6,
    abbreviate: false,
  })
}
