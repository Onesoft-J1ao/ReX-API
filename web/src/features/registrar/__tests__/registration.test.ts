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
import { describe, expect, it } from 'vitest'

import { suggestedCount, toDraft } from '../lib/draft'
import type { RegistrarProvider, RegistrarProvidersResponse } from '../types'

const provider: RegistrarProvider = {
  id: 'dahl',
  label: 'Dahl Inference',
  enabled: true,
  script_path: '/opt/registrar/dahl/runner.py',
  channel_id: 30,
  min_keys: 50,
  target_keys: 200,
  max_batch: 100,
  workers: 8,
  use_proxy: false,
  account_count: 235,
  channel: {
    id: 30,
    name: 'dahl-free',
    multi_key: true,
    total_keys: 235,
    usable_keys: 40,
    group: 'free',
  },
}

describe('toDraft', () => {
  it('drops the server-only counters and renames the interval', () => {
    const response: RegistrarProvidersResponse = {
      enabled: true,
      interval_minutes: 10,
      providers: [provider],
    }

    const draft = toDraft(response)

    expect(draft.enabled).toBe(true)
    expect(draft.intervalMinutes).toBe(10)
    expect(draft.providers).toEqual([
      {
        id: 'dahl',
        label: 'Dahl Inference',
        enabled: true,
        script_path: '/opt/registrar/dahl/runner.py',
        channel_id: 30,
        min_keys: 50,
        target_keys: 200,
        max_batch: 100,
        workers: 8,
        use_proxy: false,
      },
    ])
  })
})

describe('suggestedCount', () => {
  it('caps the gap to the target at the batch limit', () => {
    // 200 target - 40 usable = 160, above the 100 batch cap.
    expect(suggestedCount(provider)).toBe(100)
  })

  it('uses the gap when it fits under the cap', () => {
    const mostlyFull = {
      ...provider,
      channel: { ...provider.channel, usable_keys: 150 },
    }
    expect(suggestedCount(mostlyFull)).toBe(50)
  })

  it('never suggests a zero-account run when the channel is already at target', () => {
    const atTarget = {
      ...provider,
      channel: { ...provider.channel, usable_keys: 235 },
    }
    expect(suggestedCount(atTarget)).toBe(1)
  })
})
