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

/** The price dimensions the sentinel reports and the edit page may write. */
export const PRICE_SENTINEL_FIELDS = [
  'model_ratio',
  'model_price',
  'completion_ratio',
  'cache_ratio',
] as const

export type PriceSentinelField = (typeof PRICE_SENTINEL_FIELDS)[number]

/** One model whose price differs from the mirrored upstream. */
export type PriceChange = {
  model_name: string
  field: string
  /** Absent when we never configured the model, which reads as "they charge". */
  ours: number | null
  theirs: number | null
  /** Signed change from ours to theirs; positive means the upstream is dearer. */
  percent: number
  groups?: string[]
}

export type PriceSentinelReport = {
  id: number
  checked_at: number
  upstream_url: string
  compared: number
  changed: number
  higher_price: number
  lower_price: number
  fetch_error?: string
  email_sent_at: number
}

/** What the mailed link unlocks, before the operator authenticates. */
export type PriceSentinelEditSession = {
  report: PriceSentinelReport | null
  changes: PriceChange[]
  expires_at: number
}

export type PriceSentinelReportResponse = {
  report: PriceSentinelReport | null
  changes: PriceChange[]
}

export type PriceSentinelApplyResponse = {
  model_name: string
  field: string
  value: number
}
