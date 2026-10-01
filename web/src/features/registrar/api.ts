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
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  RegistrarAccountsResponse,
  RegistrarImportResult,
  RegistrarProvidersResponse,
  RegistrarRunResult,
  RegistrarSettingsPayload,
} from './types'

/** Reads every configured provider with its live account and channel counts. */
export async function getRegistrarProviders(): Promise<RegistrarProvidersResponse> {
  const response = requireServerSuccess(
    (await api.get<{ success: boolean; data: RegistrarProvidersResponse }>(
      '/api/registrar/providers'
    )).data
  )
  return response.data as RegistrarProvidersResponse
}

/** Persists the provider list, the master switch, and the check interval. */
export async function updateRegistrarProviders(
  payload: RegistrarSettingsPayload
): Promise<void> {
  requireServerSuccess(
    (await api.put<{ success: boolean }>('/api/registrar/providers', payload))
      .data
  )
}

/** Queues one manual registration run for a provider. */
export async function runRegistrarProvider(
  providerId: string,
  count: number
): Promise<RegistrarRunResult> {
  const response = requireServerSuccess(
    (await api.post<{ success: boolean; data: RegistrarRunResult }>(
      `/api/registrar/providers/${encodeURIComponent(providerId)}/run`,
      { count }
    )).data
  )
  return response.data as RegistrarRunResult
}

/** Lists archived accounts. Credentials stay masked unless `reveal` is set. */
export async function listRegistrarAccounts(params: {
  providerId: string
  page?: number
  pageSize?: number
  reveal?: boolean
}): Promise<RegistrarAccountsResponse> {
  const response = requireServerSuccess(
    (await api.get<{ success: boolean; data: RegistrarAccountsResponse }>(
      '/api/registrar/accounts',
      {
        params: {
          provider: params.providerId,
          page: params.page ?? 1,
          page_size: params.pageSize ?? 50,
          ...(params.reveal ? { reveal: '1' } : {}),
        },
      }
    )).data
  )
  return response.data as RegistrarAccountsResponse
}

/** Folds an existing account archive into the store, skipping known keys. */
export async function importRegistrarAccounts(
  providerId: string,
  content: string
): Promise<RegistrarImportResult> {
  const response = requireServerSuccess(
    (await api.post<{ success: boolean; data: RegistrarImportResult }>(
      '/api/registrar/accounts/import',
      { provider_id: providerId, content }
    )).data
  )
  return response.data as RegistrarImportResult
}

/**
 * Downloads the raw account log as a file. The route needs the session's
 * credentials, so this goes through the shared client rather than a plain link.
 */
export async function downloadRegistrarAccounts(
  providerId: string
): Promise<void> {
  const response = await api.get<Blob>('/api/registrar/accounts/export', {
    params: { provider: providerId },
    responseType: 'blob',
  })
  const url = URL.createObjectURL(response.data)
  const link = document.createElement('a')
  link.href = url
  link.download = `${providerId}-accounts.jsonl`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
