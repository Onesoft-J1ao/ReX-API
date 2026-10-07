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
import { useQuery } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'

import {
  getBangumiSourceSubject,
  getBangumiSources,
  resolveBangumiSource,
  searchBangumiSource,
} from '../api'
import { readPreference, writePreference } from '../lib/player-preference'
import type {
  BangumiMediaSelection,
  BangumiMediaSource,
  BangumiSourceChannel,
  BangumiSourceState,
} from '../types'

/** Where the last source that actually played is remembered. */
export const LAST_SOURCE_KEY = 'bangumi:player:last-source'

/**
 * How many sources an automatic pick may try before giving up.
 *
 * Every probe spends upstream requests on the visitor's behalf, and a source
 * that fails on its own merits rarely becomes the one that works after the
 * first few; the bound is what keeps a page load from walking the whole
 * catalogue.
 */
const AUTO_PROBE_LIMIT = 6

export type AutoSelectionState = 'idle' | 'working' | 'ready' | 'unavailable'

/**
 * Why no source could be picked, in the terms the viewer needs:
 * `network` means the instance could not reach the sources at all, `blocked`
 * means they answered but wanted a check only a browser can pass, and `empty`
 * means they answered with nothing to play.
 */
export type AutoSelectionFailure = 'network' | 'blocked' | 'empty'

type ProbeResult =
  | { ok: true; selection: BangumiMediaSelection }
  | { ok: false; reason: AutoSelectionFailure }

function failureFor(state: BangumiSourceState): AutoSelectionFailure {
  switch (state) {
    case 'captcha_required':
    case 'browser_required':
    case 'invalid_config':
    case 'rate_limited':
    case 'selection_required':
      return 'blocked'
    case 'empty':
      return 'empty'
    default:
      return 'network'
  }
}

/**
 * The episode to start on: the first one, preferring the one numbered 1.
 *
 * The number is a string on the wire ("1", "SP", "剧场版"), so a numeric match
 * is the only one that means what it says.
 */
function firstEpisode(channels: BangumiSourceChannel[]) {
  const episodes = channels.flatMap((channel) => channel.episodes)
  if (episodes.length === 0) return null
  return (
    episodes.find((episode) => episode.episodeNumber.trim() === '1') ??
    episodes[0]
  )
}

async function probeSource(
  source: BangumiMediaSource,
  title: string,
  signal: AbortSignal
): Promise<ProbeResult> {
  const search = await searchBangumiSource(source.id, title, signal)
  if (search.state !== 'ready' || search.subjects.length === 0) {
    return { ok: false, reason: failureFor(search.state) }
  }
  const browse = await getBangumiSourceSubject(
    source.id,
    search.subjects[0].id,
    signal
  )
  if (browse.state !== 'ready') {
    return { ok: false, reason: failureFor(browse.state) }
  }
  const episode = firstEpisode(browse.channels)
  if (!episode) return { ok: false, reason: 'empty' }

  const resolved = await resolveBangumiSource(source.id, episode.id, signal)
  if (resolved.state !== 'ready' || !resolved.streamUrl) {
    return { ok: false, reason: failureFor(resolved.state) }
  }
  const channel = browse.channels.find((item) =>
    item.episodes.some((item) => item.id === episode.id)
  )
  return {
    ok: true,
    selection: {
      sourceId: source.id,
      sourceName: source.name,
      subjectName: search.subjects[0].name,
      channelId: channel?.id ?? '',
      channelName: channel?.label || channel?.name || '',
      episode,
      streamUrl: resolved.streamUrl,
      format: resolved.format,
    },
  }
}

/**
 * Picks a source and an episode for a subject without the viewer asking.
 *
 * The order is the source that last played, then the catalogue order, which is
 * by tier: the cheapest and most reliable first. Every candidate is probed all
 * the way to a playable stream rather than trusted on its search result,
 * because a source whose search answers can still be blocked at its player
 * page, and the point of this is to land on something that actually plays.
 */
export function useAutoSourceSelection(options: {
  title: string
  enabled: boolean
  onSelected: (selection: BangumiMediaSelection) => void
}) {
  const { title, enabled, onSelected } = options
  const [state, setState] = useState<AutoSelectionState>('idle')
  const [sourceId, setSourceId] = useState<string | null>(null)
  const [firstCandidateId, setFirstCandidateId] = useState<string | null>(null)
  const [failure, setFailure] = useState<AutoSelectionFailure | null>(null)
  const [attempt, setAttempt] = useState(0)
  // A source whose stream the browser could not actually play. A resolve that
  // succeeds only proves the server could reach the media; the browser can still
  // fail to decode it, and that is what makes the pick try the next one.
  const [skipped, setSkipped] = useState<string[]>([])
  const selected = useRef(onSelected)
  selected.current = onSelected

  const catalog = useQuery({
    queryKey: ['bangumi', 'sources'],
    queryFn: () => getBangumiSources(),
    staleTime: 5 * 60 * 1000,
    meta: { errorToast: false },
  })
  const sources = catalog.data?.sources

  useEffect(() => {
    if (!enabled || !sources) return
    const candidates = sources.filter(
      (source) =>
        source.kind === 'web' &&
        source.enabled &&
        !skipped.includes(source.id)
    )
    if (candidates.length === 0) {
      setState('unavailable')
      setFailure('empty')
      return
    }
    const lastUsed = readPreference(LAST_SOURCE_KEY)
    const ordered = lastUsed
      ? [
          ...candidates.filter((source) => source.id === lastUsed),
          ...candidates.filter((source) => source.id !== lastUsed),
        ]
      : candidates

    const controller = new AbortController()
    let cancelled = false
    setFirstCandidateId(ordered[0]?.id ?? null)
    const run = async () => {
      setState('working')
      setFailure(null)
      const reasons: AutoSelectionFailure[] = []
      for (const source of ordered.slice(0, AUTO_PROBE_LIMIT)) {
        if (controller.signal.aborted) return
        try {
          const result = await probeSource(source, title, controller.signal)
          if (cancelled) return
          if (result.ok) {
            setSourceId(source.id)
            setState('ready')
            writePreference(LAST_SOURCE_KEY, source.id)
            selected.current(result.selection)
            return
          }
          reasons.push(result.reason)
        } catch {
          // A cancelled probe is not a source's fault; anything else counts as
          // unreachable, which is what the viewer is told if nothing works.
          if (cancelled || controller.signal.aborted) return
          reasons.push('network')
        }
      }
      if (cancelled) return
      setSourceId(null)
      let reason: AutoSelectionFailure = 'empty'
      if (reasons.every((value) => value === 'network')) reason = 'network'
      else if (reasons.includes('blocked')) reason = 'blocked'
      setFailure(reason)
      setState('unavailable')
    }
    void run()
    return () => {
      cancelled = true
      controller.abort()
    }
  }, [enabled, sources, title, attempt, skipped])

  const retry = useCallback(() => setAttempt((value) => value + 1), [])
  /**
   * Drop a source that resolved but did not play, and pick again.
   *
   * The failed one is excluded for the rest of the page's life, so a source
   * whose CDN is broken is not chosen twice.
   */
  const advance = useCallback((sourceId: string) => {
    setSkipped((previous) =>
      previous.includes(sourceId) ? previous : [...previous, sourceId]
    )
    setAttempt((value) => value + 1)
  }, [])
  return { state, sourceId, firstCandidateId, failure, retry, advance }
}
