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

/**
 * Today's releases. `releases` reuses the release shape the player already
 * consumes, so a card can open the same episode dialog without a second model.
 */
export type BangumiTodayUpdatesResponse = {
  date: string
  count: number
  releases: BangumiRelease[]
}

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

/** A Mikan release streamed through the instance; attributes follow Animeko's Topic model. */
export type BangumiRelease = {
  topicId: string
  title: string
  /** The release group, taken from the bracketed title prefix. */
  alliance: string
  description: string
  /** Torrent metadata is handed to the instance's streaming backend. */
  downloadLink: string
  /** A number parsed by the server; 0 denotes a batch or an unstated episode. */
  episodeNumber: number
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
  /** H264, H265, AV1 or VP9 when the title states one. */
  videoCodec?: string
  /** MP4, MKV, AVI or WebM when the title states one. */
  container?: string
  /**
   * Whether a browser can play this release as published. Decided by the server
   * so the rule is written once: H.264, in whichever container it ships in.
   */
  playable: boolean
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

export type BangumiSourceState =
  | 'idle'
  | 'working'
  | 'ready'
  | 'empty'
  | 'failed'
  | 'captcha_required'
  | 'rate_limited'
  | 'browser_required'
  | 'invalid_config'
  | 'selection_required'

export type BangumiMediaSource = {
  id: string
  name: string
  kind: 'web' | 'bt'
  icon: string
  tier: number
  enabled: boolean
  /**
   * The source's own site. Shown when a source answers with a challenge, so the
   * viewer has somewhere to go and clear it by hand.
   */
  websiteUrl?: string
}

export type BangumiSourcesResponse = {
  sources: BangumiMediaSource[]
  legacyMikanSourceId: string
}

export type BangumiSourceStatus = {
  sourceId: string
  state: BangumiSourceState
  captchaKind?: string
  retryAt?: string
  message?: string
}

export type BangumiSourceSubject = { id: string; name: string }

export type BangumiSourceSearchResponse = BangumiSourceStatus & {
  subjects: BangumiSourceSubject[]
}

export type BangumiSourceEpisode = {
  id: string
  name: string
  episodeNumber: string
}

export type BangumiSourceChannel = {
  id: string
  name: string
  label: string
  episodes: BangumiSourceEpisode[]
}

export type BangumiSourceBrowseResponse = BangumiSourceStatus & {
  channels: BangumiSourceChannel[]
}

export type BangumiMediaFormat =
  | 'hls'
  | 'mp4'
  | 'matroska'
  | 'unknown'
  | 'torrent'

export type BangumiSourceResolveResponse = BangumiSourceStatus & {
  streamUrl?: string
  format?: BangumiMediaFormat
  files?: { id: string; name: string; size: number }[]
}

export type BangumiMediaSelection = {
  sourceId: string
  sourceName: string
  subjectName: string
  channelId: string
  channelName: string
  episode: BangumiSourceEpisode
  streamUrl: string
  format?: BangumiMediaFormat
}
