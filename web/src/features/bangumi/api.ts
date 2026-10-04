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
  BangumiDanmakuResponse,
  BangumiEpisodesResponse,
  BangumiReleasesResponse,
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

/**
 * Rewrite a Mikan torrent URL onto this instance's torrent proxy.
 *
 * The browser cannot fetch the .torrent itself: Mikan serves it without CORS
 * headers, so the request is blocked before WebTorrent ever sees the bytes. The
 * proxy accepts only Mikan's own /Download/ path, so the host is discarded and
 * only that prefix is kept.
 */
export function bangumiTorrentUrl(downloadLink: string): string | null {
  const path = downloadLink.replace(/^https?:\/\/[^/]+/, '').replace(/^\/+/, '')
  return path.startsWith('Download/') ? `/api/bangumi/torrent/${path}` : null
}

/**
 * The watchable episodes of a subject, from dandanplay.
 *
 * dandanplay indexes the same subject ids bgm.tv uses, so the id the page
 * already holds is enough.
 */
export async function getBangumiEpisodes(
  subjectId: number | string
): Promise<BangumiEpisodesResponse> {
  const res = await api.get<BangumiEpisodesResponse>(
    `/api/bangumi/subjects/${subjectId}/episodes`
  )
  return res.data
}

/**
 * The bullet comments for one episode.
 *
 * Each comment's `p` is left as dandanplay sends it; only the renderer can
 * split "time,mode,color,userHash" meaningfully.
 */
export async function getBangumiDanmaku(
  episodeId: number | string
): Promise<BangumiDanmakuResponse> {
  const res = await api.get<BangumiDanmakuResponse>(
    `/api/bangumi/danmaku/${episodeId}`
  )
  return res.data
}

/**
 * The Mikan releases for a subject.
 *
 * The keyword is required because Mikan has no subject-id lookup: the server
 * searches by name and then confirms the match against Mikan's own page, so the
 * title the page already holds is enough to resolve it.
 */
export async function getBangumiReleases(
  subjectId: number | string,
  keyword: string
): Promise<BangumiReleasesResponse> {
  const res = await api.get<BangumiReleasesResponse>(
    `/api/bangumi/releases/${subjectId}`,
    { params: { keyword } }
  )
  return res.data
}
