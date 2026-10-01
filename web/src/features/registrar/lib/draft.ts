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
import type {
  RegistrarProvider,
  RegistrarProviderConfig,
  RegistrarProvidersResponse,
} from '../types'

export interface RegistrarDraft {
  enabled: boolean
  intervalMinutes: number
  providers: RegistrarProviderConfig[]
}

/**
 * Keeps only the fields the settings endpoint accepts: the account counter and
 * the channel snapshot are read-only state the server reports back.
 */
export function toDraft(data: RegistrarProvidersResponse): RegistrarDraft {
  return {
    enabled: data.enabled,
    intervalMinutes: data.interval_minutes,
    providers: data.providers.map(
      ({ account_count: _accountCount, channel: _channel, ...config }) => config
    ),
  }
}

/** How many accounts a top-up needs right now, used as the run dialog default. */
export function suggestedCount(provider: RegistrarProvider): number {
  const missing = provider.target_keys - provider.channel.usable_keys
  if (missing <= 0) return 1
  return Math.min(missing, Math.max(provider.max_batch, 1))
}
