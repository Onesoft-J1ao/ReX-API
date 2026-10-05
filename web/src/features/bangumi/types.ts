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

/** 2 is the upstream's type for anime; the page never asks for anything else. */
export type BangumiSubjectType = 2

export type BangumiImages = {
  large?: string
  common?: string
  medium?: string
  small?: string
  grid?: string
}

export type BangumiRating = {
  rank?: number
  total?: number
  score?: number
  count?: Record<string, number>
}

export type BangumiCollection = {
  collect?: number
  doing?: number
  wish?: number
  on_hold?: number
  dropped?: number
}

/** One entry in the daily broadcast calendar. */
export type BangumiCalendarItem = {
  id: number
  url?: string
  type: BangumiSubjectType
  name: string
  /** Empty for subjects with no Chinese title; fall back to `name`. */
  name_cn?: string
  summary?: string
  air_date?: string
  /** 1 is Monday, 7 is Sunday. */
  air_weekday?: number
  images?: BangumiImages
  rating?: BangumiRating
  collection?: BangumiCollection
}

export type BangumiCalendarDay = {
  weekday: { en?: string; cn?: string; ja?: string; id: number }
  items: BangumiCalendarItem[]
}

export type BangumiCalendarResponse = BangumiCalendarDay[]

export type BangumiSearchItem = {
  id: number
  type: BangumiSubjectType
  name: string
  name_cn?: string
  summary?: string
  date?: string
  images?: BangumiImages
  rating?: BangumiRating
  collection?: BangumiCollection
  tags?: { name: string; count?: number }[]
}

export type BangumiSearchResponse = {
  data?: BangumiSearchItem[]
  total?: number
  limit?: number
  offset?: number
}

export type BangumiTag = {
  name: string
  count?: number
  total_count?: number
}

export type BangumiInfoboxEntry = {
  key?: string
  value?: string
  k?: string
  v?: string
}

/** The `/v0/subjects/{id}` payload. */
export type BangumiSubject = {
  id: number
  type: BangumiSubjectType
  name: string
  name_cn?: string
  summary?: string
  date?: string
  platform?: string
  /** 0 while the upstream has not counted the episodes yet. */
  eps?: number
  total_episodes?: number
  images?: BangumiImages
  rating?: BangumiRating
  collection?: BangumiCollection
  tags?: BangumiTag[]
  meta_tags?: BangumiTag[]
  infobox?: BangumiInfoboxEntry[]
  series?: boolean
  volumes?: number
  locked?: boolean
  nsfw?: boolean
}

/**
 * A BitTorrent release from Mikan Project, with the attributes parsed from its
 * title.
 *
 * Mikan publishes torrents rather than video files, so a release is only ever
 * played by the visitor's own browser joining the swarm. The attribute names
 * follow Animeko's Topic vocabulary so the two agree on what a release is.
 */
export type BangumiRelease = {
  topicId: string
  title: string
  /** The release group, taken from the bracketed title prefix. */
  alliance: string
  description: string
  /** The .torrent URL, which is what the browser hands to WebTorrent. */
  downloadLink: string
  episodeLink?: string
  /** Size in bytes; 0 when the feed did not state one. */
  size: number
  publishedAt: string
  /** Unix milliseconds; 0 when the feed did not state a date. */
  publishedTime: number
  /** A resolution id such as 1080P, or an empty string when unstated. */
  resolution: string
  mediaOrigin?: string
  /** Subtitle languages as CHS/CHT/CHC/JPN/ENG. */
  subtitleLanguages: string[]
  /** EMBEDDED, CLOSED or EXTERNAL_DISCOVER when the title states one. */
  subtitleKind?: string
  /**
   * Always `EXACT`: the server confirms a candidate against Mikan's own page
   * before returning its releases, so a name-only match is dropped rather than
   * reported.
   */
  matchKind: string
}

/** One episode, with the release the server chose for it when one matched. */
export type BangumiEpisode = {
  episodeId: number
  episodeTitle: string
  episodeNumber: string
  airDate?: string
  release?: BangumiRelease
}

export type BangumiEpisodesResponse = {
  animeId: number
  animeTitle: string
  episodes: BangumiEpisode[]
}

export type BangumiReleasesResponse = {
  subjectId: number
  matchKind: string
  releases: BangumiRelease[]
  episodes: BangumiEpisode[]
}

/** One bullet comment, exactly as dandanplay serves it. */
export type BangumiDanmakuComment = {
  cid: number
  /** "timeSeconds,mode,color,userHash". */
  p: string
  m: string
}

export type BangumiDanmakuResponse = {
  count: number
  comments: BangumiDanmakuComment[]
}
