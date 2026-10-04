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
  BangumiCalendarResponse,
  BangumiSearchResponse,
  BangumiSubject,
} from './types'

/**
 * Anime metadata proxied through this instance rather than fetched from bgm.tv
 * directly: the upstream asks callers to identify themselves and refuses
 * generic browser agents, and one proxy is a single place to cache and limit.
 */
export async function getBangumiCalendar(): Promise<BangumiCalendarResponse> {
  const res = await api.get<BangumiCalendarResponse>('/api/bangumi/calendar')
  return res.data
}

export async function searchBangumi(
  keyword: string,
  limit = 20
): Promise<BangumiSearchResponse> {
  const res = await api.get<BangumiSearchResponse>('/api/bangumi/search', {
    params: { keyword, limit },
  })
  return res.data
}

export async function getBangumiSubject(
  subjectId: number | string
): Promise<{ data: BangumiSubject }> {
  const res = await api.get<{ data: BangumiSubject }>(
    `/api/bangumi/subjects/${subjectId}`
  )
  return res.data
}

/**
 * Rewrite an upstream cover URL onto this instance's image proxy.
 *
 * The upstream hands out two shapes and the proxy forwards each verbatim: a
 * bare `/pic/cover/l/...` from the calendar, and an already-sized
 * `/r/400/pic/...` from the subject endpoint. Stripping `/r/` would break the
 * second one -- its image host only serves the sized form under that prefix.
 * The calendar's form also arrives as `http://`, which a browser on an HTTPS
 * page refuses to load, so only the scheme is normalized away here.
 */
export function bangumiCoverUrl(
  images: Record<string, string> | undefined,
  size: 'large' | 'common' | 'medium' | 'small' | 'grid' = 'common'
): string | null {
  const raw = images?.[size] ?? images?.common ?? images?.medium
  if (!raw) return null

  const path = raw.replace(/^https?:\/\/[^/]+/, '').replace(/^\/+/, '')
  return path ? `/api/bangumi-image/${path}` : null
}
