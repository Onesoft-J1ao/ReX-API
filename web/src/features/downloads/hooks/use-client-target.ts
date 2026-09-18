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
import { useEffect, useState } from 'react'

import {
  detectClientTarget,
  readDetectionSignals,
  readHighEntropyArch,
  readUserAgentData,
} from '../lib/platform'
import type { ClientTarget } from '../types'

/**
 * Detect the visitor's system, refining the result once the browser reports
 * its high-entropy architecture hint.
 *
 * The synchronous pass already resolves the platform correctly; the async
 * upgrade exists for Windows on ARM, where the User-Agent claims x64 because
 * of emulation.
 */
export function useClientTarget(): ClientTarget {
  const [target, setTarget] = useState<ClientTarget>(() =>
    detectClientTarget(readDetectionSignals())
  )

  useEffect(() => {
    let cancelled = false

    void readHighEntropyArch(readUserAgentData()).then((arch) => {
      if (cancelled || !arch) return

      setTarget((current) => (current.arch === arch ? current : { ...current, arch }))
    })

    return () => {
      cancelled = true
    }
  }, [])

  return target
}
