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
import { api } from '@/lib/api'

export type RedemptionPurchaseOptions = {
  enabled: boolean
  /** Payment methods this deployment can actually take, decided by the server. */
  methods: string[]
  max_count: number
  max_amount: number
  min_amount: number
}

export type RedemptionPurchasePayload = {
  unit_amount: number
  quantity: number
  payment_method: string
}

export type RedemptionPurchaseResult = {
  message: string
  /** epay returns form parameters plus the URL to post them to. */
  data?: Record<string, unknown> | { pay_link?: string }
  url?: string
}

export async function getRedemptionPurchaseOptions(): Promise<RedemptionPurchaseOptions> {
  const res = await api.get('/api/user/self/redemption/purchase')
  return res.data.data as RedemptionPurchaseOptions
}

/** The money one order would cost, asked before the buyer commits to it. */
export async function getRedemptionPurchaseAmount(
  payload: RedemptionPurchasePayload
): Promise<string> {
  const res = await api.post('/api/user/self/redemption/purchase/amount', payload)
  return String(res.data.data)
}

export async function requestRedemptionPurchase(
  payload: RedemptionPurchasePayload
): Promise<RedemptionPurchaseResult> {
  const res = await api.post('/api/user/self/redemption/purchase', payload)
  return res.data as RedemptionPurchaseResult
}
