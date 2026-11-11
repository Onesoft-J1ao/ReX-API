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
import { FILTER_ALL } from '../constants'

/**
 * The fields the resolver reads. A reading may carry `source`, and `mirror`
 * marks one measured by the mirrored upstream — another deployment's blended
 * figure, which has no group dimension of its own.
 */
export type ModelPerfReading = {
  source?: string
  /**
   * False when the reading describes the model across all its groups rather
   * than the group on screen — the card fell back to the model-wide rollup
   * because this group carried no traffic for the model. Absent means the
   * reading belongs to the group it is shown in.
   */
  groupScoped?: boolean
}

/**
 * The three places a card's health can come from, each keyed the way its
 * source keys it.
 */
export type ModelPerfIndex<T extends ModelPerfReading> = {
  /** Active probe rows, keyed `group|model`. Freshest signal, and ours. */
  probeByGroup: Map<string, T>
  /** Passive rollup split by group, keyed `group|model`. Ours as well. */
  rollupByGroup: Map<string, T>
  /** Model-wide rollup, keyed by model name. May be `source: 'mirror'`. */
  modelWide: Map<string, T>
}

/**
 * True when a reading came from another deployment's mirror rather than from
 * this one. `source` is "mirror" on the model-wide rollup and
 * "mirror:<source>" on a group row.
 */
function isMirrored(reading: ModelPerfReading): boolean {
  return reading.source?.startsWith('mirror') === true
}

/**
 * The first reading this deployment measured for a model in any group.
 *
 * Used only for the unfiltered square, where a card stands for the model as a
 * whole. A probe is our own request through our own router, so it is a better
 * answer to "is this model up for us" than another instance's blended figure.
 */
function firstOwnReading<T extends ModelPerfReading>(
  index: ModelPerfIndex<T>,
  modelName: string
): T | undefined {
  const suffix = `|${modelName}`
  for (const [key, value] of index.probeByGroup) {
    if (key.endsWith(suffix) && !isMirrored(value)) return value
  }
  for (const [key, value] of index.rollupByGroup) {
    if (key.endsWith(suffix) && !isMirrored(value)) return value
  }
  return undefined
}

/**
 * Pick the reading a card should show.
 *
 * With a group selected, our own data for that group wins in order of
 * freshness: the probe, then the passive rollup. A model this deployment
 * carried no traffic for in that group has neither, so the model-wide figure is
 * the only number available. It is shown rather than blanked — a card with no
 * reading at all tells a buyer nothing — but it is marked `groupScoped: false`
 * so the card can say it describes the model, not the group.
 *
 * That mark matters when a model is routed through several groups and none of
 * them has data: every one of those cards falls back to the same model-wide
 * figure, and without the mark two different groups look like they have
 * identical health. The numbers really are identical — they are not this
 * group's numbers at all.
 *
 * Without a group the card stands for the model as a whole, so the model-wide
 * figure is the right one: it is the rollup across every group the model is
 * routed through. Reaching into whichever group happens to come first would
 * show that one group's uptime as the model's — on production that turned a
 * model measured at 100% into 0% because an unrelated group was listed first.
 * A probe reading is used only when there is no model-wide figure at all.
 */
export function resolveModelPerf<T extends ModelPerfReading>(
  index: ModelPerfIndex<T>,
  modelName: string,
  selectedGroup?: string
): T | undefined {
  if (!modelName) return undefined
  if (selectedGroup && selectedGroup !== FILTER_ALL) {
    const key = `${selectedGroup}|${modelName}`
    const own = index.probeByGroup.get(key) ?? index.rollupByGroup.get(key)
    if (own) return own
    const wide = index.modelWide.get(modelName)
    if (!wide) return undefined
    // Borrowed from the model-wide rollup: it says nothing about this group.
    return { ...wide, groupScoped: false }
  }
  const wide = index.modelWide.get(modelName)
  if (wide && !isMirrored(wide)) return wide
  // The model-wide figure is another deployment's number. The unfiltered
  // square used to show it unconditionally, so every model whose upstream runs
  // at ~60% painted the whole page red while our own probes read 98-100%.
  const own = firstOwnReading(index, modelName)
  if (own) return own
  if (wide) return wide
  return undefined
}
