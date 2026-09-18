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

import type {
  AdminRefundListParams,
  ApiResponse,
  RefundAdvanceAction,
  RefundListParams,
  RefundListResponse,
  RefundRequest,
} from './types'

// ============================================================================
// Refund Requests
// ============================================================================

// The caller's own refund requests, so the user can watch their progress.
export async function getSelfRefunds(
  params: RefundListParams = {}
): Promise<RefundListResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/api/refund/self?p=${p}&page_size=${page_size}`)
  return res.data
}

// Every refund request, optionally narrowed to one status (admin).
export async function getRefunds(
  params: AdminRefundListParams = {}
): Promise<RefundListResponse> {
  const { status = '', p = 1, page_size = 10 } = params
  const query = new URLSearchParams()
  if (status) query.set('status', status)
  query.set('p', String(p))
  query.set('page_size', String(page_size))
  const res = await api.get(`/api/refund/admin/?${query.toString()}`)
  return res.data
}

// Move a request forward: pending -> approved, approved -> credited, or
// pending|approved -> rejected.
export async function advanceRefund(
  id: number,
  status: RefundAdvanceAction
): Promise<ApiResponse<RefundRequest>> {
  const res = await api.post(`/api/refund/admin/${id}/advance`, { status })
  return res.data
}
