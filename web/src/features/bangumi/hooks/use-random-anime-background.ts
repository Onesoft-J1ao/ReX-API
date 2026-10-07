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

import { useSystemConfig } from '@/hooks/use-system-config'

/** Nekosia returns one random image per request for a given category slug. */
const NEKOSIA_ENDPOINT = 'https://api.nekosia.cat/api/v1/images'

type NekosiaResponse = {
  image?: {
    original?: { url?: string }
    compressed?: { url?: string }
  }
}

/**
 * Resolves a random anime backdrop for the anime pages.
 *
 * The category comes from `console_setting.background_random_category`; an
 * empty value disables the remote lookup and the caller falls back to the
 * bundled pool. The request is best-effort: a failure or a slow upstream
 * simply leaves `url` empty rather than blocking the page.
 */
export function useRandomAnimeBackground(): {
  url: string
  /** A category is configured but its image has not arrived yet. */
  pending: boolean
} {
  const { appearance } = useSystemConfig()
  const category = appearance.backgroundRandomCategory
  const [url, setUrl] = useState('')
  const [pending, setPending] = useState(false)

  useEffect(() => {
    if (!category) {
      setUrl('')
      setPending(false)
      return
    }
    // The request is in flight: the caller holds the backdrop empty rather
    // than showing the bundled landscape for a beat and swapping it out.
    setPending(true)

    const controller = new AbortController()
    let cancelled = false

    const load = async () => {
      try {
        const res = await fetch(
          `${NEKOSIA_ENDPOINT}/${encodeURIComponent(category)}`,
          { signal: controller.signal }
        )
        if (!res.ok) return
        const data: NekosiaResponse = await res.json()
        const resolved =
          data.image?.original?.url || data.image?.compressed?.url || ''
        if (!cancelled && resolved) {
          setUrl(resolved)
        }
      } catch {
        // Offline, blocked, or aborted: keep the bundled fallback.
      } finally {
        if (!cancelled) setPending(false)
      }
    }

    void load()
    return () => {
      cancelled = true
      controller.abort()
    }
  }, [category])

  return { url, pending }
}
