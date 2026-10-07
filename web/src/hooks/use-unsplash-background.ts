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

/** How often the backdrop is replaced. The server caches for five minutes,
 * so a longer client interval always receives a fresh photo. */
const UNSPLASH_REFRESH_MS = 10 * 60 * 1000

import { api } from '@/lib/api'

/** Attribution the Unsplash guidelines require alongside the image. */
export type BackgroundCredit = {
  authorName: string
  authorLink: string
  photoLink: string
}

export type RandomBackground = BackgroundCredit & { url: string }

type RandomBackgroundResponse = {
  success?: boolean
  data?: Partial<RandomBackground> | null
}

function normalizeCredit(data: RandomBackground): BackgroundCredit | null {
  if (!data.authorName) return null
  return {
    authorName: data.authorName,
    authorLink: data.authorLink,
    photoLink: data.photoLink,
  }
}

/**
 * Loads the operator-configured Unsplash backdrop.
 *
 * The request goes through our own API so the access key stays server-side;
 * the browser only ever sees the resolved image URL and its credit. An
 * unconfigured site returns no data, and the caller keeps its bundled pool.
 */
export function useUnsplashBackground(enabled = true): {
  image: string
  credit: BackgroundCredit | null
  /** The lookup is in flight and no image has been shown yet. */
  pending: boolean
} {
  const [image, setImage] = useState('')
  const [credit, setCredit] = useState<BackgroundCredit | null>(null)
  const [pending, setPending] = useState(false)

  useEffect(() => {
    // A page that supplies its own image never shows this one, so asking the
    // server for it would only spend a request.
    if (!enabled) return

    let cancelled = false
    setPending(true)

    const load = async () => {
      try {
        const res = await api.get<RandomBackgroundResponse>(
          '/api/background/random'
        )
        const data = res.data?.data
        if (cancelled || !data?.url) return
        setImage(data.url)
        setCredit(normalizeCredit(data as RandomBackground))
      } catch {
        // Unconfigured or unreachable: the caller falls back to bundled images.
      } finally {
        if (!cancelled) setPending(false)
      }
    }

    void load()
    // The backdrop rotates on its own; the viewer does not have to reload.
    const timer = setInterval(load, UNSPLASH_REFRESH_MS)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [enabled])

  return { image, credit, pending }
}
