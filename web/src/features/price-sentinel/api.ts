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
import axios from 'axios'

import { api } from '@/lib/api'

import type {
  PriceSentinelApplyResponse,
  PriceSentinelEditSession,
  PriceSentinelReportResponse,
} from './types'

/**
 * The edit page is reached from an emailed link and authenticated with a token
 * the operator pastes in, so it must not ride the shared client: that client
 * overwrites `Authorization` with the browser session, which would replace a
 * pasted root token with whatever account happens to be signed in. This client
 * sends exactly the credential the page was given, and carries cookies so a
 * root session still works when the operator is already signed in.
 */
const editClient = axios.create({
  baseURL: '',
  withCredentials: true,
  headers: { 'Cache-Control': 'no-cache, no-store' },
})

/** Checks a mailed link and returns the report it unlocks. Needs no session. */
export async function validatePriceSentinelToken(
  token: string
): Promise<PriceSentinelEditSession> {
  const res = await editClient.post('/api/price-sentinel/edit/validate', null, {
    params: { token },
  })
  return res.data?.data as PriceSentinelEditSession
}

/**
 * Writes one price. The mailed token proves the link, and the pasted admin
 * access token is what authorises the write; both are required.
 */
export async function applyPriceSentinelEdit(params: {
  token: string
  adminToken: string
  modelName: string
  field: string
  value: number
}): Promise<PriceSentinelApplyResponse> {
  const res = await editClient.post(
    '/api/price-sentinel/edit/apply',
    {
      token: params.token,
      model_name: params.modelName,
      field: params.field,
      value: params.value,
    },
    {
      headers: params.adminToken
        ? { Authorization: `Bearer ${params.adminToken}` }
        : undefined,
    }
  )
  return res.data?.data as PriceSentinelApplyResponse
}

/** Reads the newest report for the admin console. Requires a root session. */
export async function getPriceSentinelReport(): Promise<PriceSentinelReportResponse> {
  const res = await api.get('/api/price-sentinel/report')
  return res.data?.data as PriceSentinelReportResponse
}

/** Runs a check on demand, without waiting for the hourly task. */
export async function runPriceSentinelNow(): Promise<void> {
  await api.post('/api/price-sentinel/run')
}
