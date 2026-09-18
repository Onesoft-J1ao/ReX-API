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
import { z } from 'zod'

// ============================================================================
// Refund Request Schema & Types
// ============================================================================

export const refundRequestSchema = z.object({
  id: z.number(),
  user_id: z.number(),
  // 'plan' converts a subscription's remaining allowance back into wallet
  // balance; 'cash' pays out a share of the withdrawable balance.
  kind: z.string(),
  status: z.string(),
  user_subscription_id: z.number().optional().default(0),
  // Principal the refund was computed from, the balance actually removed, and
  // the amount refunded after the applicable ratio.
  principal_quota: z.number(),
  deducted_quota: z.number(),
  quota: z.number(),
  amount: z.number(),
  remark: z.string().optional().default(''),
  admin_id: z.number().optional().default(0),
  created_at: z.number(),
  updated_at: z.number().optional().default(0),
  processed_at: z.number().optional().default(0),
  credited_at: z.number().optional().default(0),
})

export type RefundRequest = z.infer<typeof refundRequestSchema>

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface RefundListParams {
  p?: number
  page_size?: number
}

export interface AdminRefundListParams extends RefundListParams {
  status?: string
}

export interface RefundListResponse {
  success: boolean
  message?: string
  data?: {
    items: RefundRequest[]
    total: number
    page: number
    page_size: number
  }
}

// The three states an administrator can move a request into. The backend owns
// which transitions are legal from the current state.
export type RefundAdvanceAction = 'approved' | 'credited' | 'rejected'

// ============================================================================
// Dialog Types
// ============================================================================

export type RefundsDialogType = 'advance'
