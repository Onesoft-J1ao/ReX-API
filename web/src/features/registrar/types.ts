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

/** One registration source and the channel its keys feed. */
export interface RegistrarProviderConfig {
  id: string
  label: string
  enabled: boolean
  script_path: string
  channel_id: number
  min_keys: number
  target_keys: number
  max_batch: number
  workers: number
  use_proxy: boolean
}

export interface RegistrarChannelStatus {
  id: number
  name: string
  multi_key: boolean
  total_keys: number
  usable_keys: number
  group: string
}

export interface RegistrarProvider extends RegistrarProviderConfig {
  account_count: number
  channel: RegistrarChannelStatus
}

export interface RegistrarProvidersResponse {
  enabled: boolean
  interval_minutes: number
  providers: RegistrarProvider[]
}

export interface RegistrarSettingsPayload {
  enabled: boolean
  interval_minutes: number
  providers: RegistrarProviderConfig[]
}

export interface RegistrarAccount {
  username: string
  /** Masked unless the request asked to reveal credentials. */
  fingerprint: string
  /** Masked unless the request asked to reveal credentials. */
  api_key: string
  created_at: number
  channel_id: number
}

export interface RegistrarAccountsResponse {
  items: RegistrarAccount[]
  total: number
  page: number
  page_size: number
  masked: boolean
}

export interface RegistrarRunResult {
  created: boolean
  task: { task_id: string; status: string }
}

export interface RegistrarImportResult {
  imported: number
  skipped: number
}
