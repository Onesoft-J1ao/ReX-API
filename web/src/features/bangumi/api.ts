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
  BangumiSourceBrowseResponse,
  BangumiSourceResolveResponse,
  BangumiSourceSearchResponse,
  BangumiSourcesResponse,
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

/**
 * One page of the anime ranking.
 *
 * bgm.tv orders its own subjects by rank, and the server proxies that list; the
 * page is a page number rather than an offset so the bound stays server-side.
 */
export async function getBangumiRanking(
  page = 1
): Promise<BangumiSearchResponse> {
  const res = await api.get<BangumiSearchResponse>('/api/bangumi/ranking', {
    params: { page },
  })
  return res.data
}

/**
 * One subject with its episode list.
 *
 * The instance returns the record itself, not a `{data: ...}` envelope, so the
 * caller gets the subject directly.
 */
export async function getBangumiSubject(
  subjectId: number | string
): Promise<BangumiSubject> {
  const res = await api.get<BangumiSubject>(`/api/bangumi/subjects/${subjectId}`)
  return res.data
}

/**
 * The widths the image host will actually render.
 *
 * Verified against it: it serves these six and answers 400 for anything else,
 * including a sensible-looking 320 or 640. A width outside the ladder is not a
 * smaller image, it is no image, so callers ask for a size and get snapped up
 * to the next one that exists rather than a broken request.
 */
const BANGUMI_COVER_WIDTHS = [100, 200, 400, 600, 800, 1200] as const

function bangumiCoverWidth(requested: number): number {
  return (
    BANGUMI_COVER_WIDTHS.find((width) => width >= requested) ??
    Math.max(...BANGUMI_COVER_WIDTHS)
  )
}

/**
 * Rewrite an upstream cover URL onto this instance's image proxy.
 *
 * The upstream ships every size of the same picture, and picking the wrong one
 * is the whole story here: the `common` thumbnail is about 3.5 KB where the
 * cards draw ~200 CSS px, so it was upscaled and looked blurred. The `l`
 * original is ~58 KB and the host will downscale it to whatever width is asked
 * for, so the original is the one to take and then size.
 *
 * The size prefix only works on that original. Verified against the host: it
 * serves `/r/400/pic/cover/l/...` fine but answers 400 for the same prefix on a
 * `c` or `m` path, so a subject that has no original keeps its thumbnail
 * unsized rather than being turned into a broken request. The subject endpoint
 * also hands back URLs that already carry a prefix, which is unwrapped first so
 * the width here is the one that decides.
 *
 * The calendar's URLs arrive as `http://`, which a browser on an HTTPS page
 * refuses to load, so the scheme is normalized away.
 */
export function bangumiCoverUrl(
  images: Record<string, string> | undefined,
  width = 400
): string | null {
  // Prefer the original so it can be re-sized; fall back to whatever exists for
  // a subject the upstream has not published a large cover for.
  const raw =
    images?.large ?? images?.medium ?? images?.common ?? images?.small ?? null
  if (!raw) return null

  const path = raw
    .replace(/^https?:\/\/[^/]+/, '')
    .replace(/^\/+/, '')
    .replace(/^r\/\d+\//, '')

  if (!path.startsWith('pic/cover/l/')) {
    return `/api/bangumi-image/${path}`
  }
  return `/api/bangumi-image/r/${bangumiCoverWidth(width)}/${path}`
}

/**
 * The same-origin URL the player streams an episode from.
 *
 * The instance relays the bytes. A browser cannot join a BitTorrent swarm -- it
 * speaks HTTP and the peers speak TCP and UDP -- so the torrent is fetched and
 * served by the instance, and the player reads the result like any other video.
 * The episode number travels with the request because the file to stream is
 * chosen from the torrent's own file list, which only the server can see.
 */
export function bangumiStreamUrl(
  downloadLink: string,
  episodeNumber?: number
): string | null {
  const path = downloadLink.replace(/^https?:\/\/[^/]+/, '').replace(/^\/+/, '')
  if (!path.startsWith('Download/')) return null
  const query =
    episodeNumber == null || !Number.isFinite(episodeNumber)
      ? ''
      : `?episode=${episodeNumber}`
  return `/api/bangumi/stream/${path}${query}`
}

/**
 * The watchable episodes of a subject, from dandanplay.
 *
 * dandanplay indexes the same subject ids bgm.tv uses, so the id the page
 * already holds is enough.
 */
export async function getBangumiEpisodes(
  subjectId: number | string,
  signal?: AbortSignal
): Promise<BangumiEpisodesResponse> {
  const res = await api.get<BangumiEpisodesResponse>(
    `/api/bangumi/subjects/${subjectId}/episodes`,
    { signal }
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
  episodeId: number | string,
  signal?: AbortSignal
): Promise<BangumiDanmakuResponse> {
  const res = await api.get<BangumiDanmakuResponse>(
    `/api/bangumi/danmaku/${episodeId}`,
    { signal }
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
  keyword: string,
  signal?: AbortSignal
): Promise<BangumiReleasesResponse> {
  const res = await api.get<BangumiReleasesResponse>(
    `/api/bangumi/releases/${subjectId}`,
    { params: { keyword }, signal }
  )
  return res.data
}

export async function getBangumiSources(
  signal?: AbortSignal
): Promise<BangumiSourcesResponse> {
  const res = await api.get<BangumiSourcesResponse>('/api/bangumi/sources', {
    signal,
  })
  return res.data
}

export async function searchBangumiSource(
  sourceId: string,
  keyword: string,
  signal?: AbortSignal,
  clearance?: string
): Promise<BangumiSourceSearchResponse> {
  const res = await api.get<BangumiSourceSearchResponse>(
    `/api/bangumi/sources/${encodeURIComponent(sourceId)}/search`,
    { params: { keyword, ...(clearance ? { clearance } : {}) }, signal }
  )
  return res.data
}

export async function getBangumiSourceSubject(
  sourceId: string,
  sourceSubjectId: string,
  signal?: AbortSignal,
  clearance?: string
): Promise<BangumiSourceBrowseResponse> {
  const res = await api.get<BangumiSourceBrowseResponse>(
    `/api/bangumi/sources/${encodeURIComponent(sourceId)}/subjects/${encodeURIComponent(sourceSubjectId)}`,
    { params: clearance ? { clearance } : {}, signal }
  )
  return res.data
}

export async function resolveBangumiSource(
  sourceId: string,
  episodeId: string,
  signal?: AbortSignal,
  fileId?: string,
  clearance?: string
): Promise<BangumiSourceResolveResponse> {
  const res = await api.get<BangumiSourceResolveResponse>(
    `/api/bangumi/sources/${encodeURIComponent(sourceId)}/resolve`,
    {
      params: {
        episodeId,
        ...(fileId ? { fileId } : {}),
        ...(clearance ? { clearance } : {}),
      },
      signal,
    }
  )
  return res.data
}
