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
 * Pick the reading a card should show.
 *
 * With a group selected, our own data for that group wins in order of
 * freshness: the probe, then the passive rollup. A model this deployment
 * carried no traffic for in that group has neither, and the model-wide figure
 * is then the only number available. It is shown rather than blanked — a card
 * with no reading at all tells a buyer nothing — but it keeps its `source`
 * so the badge can mark a mirrored figure as another deployment's measurement
 * of the model, not of the group on screen.
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
    return (
      index.probeByGroup.get(key) ??
      index.rollupByGroup.get(key) ??
      index.modelWide.get(modelName)
    )
  }
  const wide = index.modelWide.get(modelName)
  if (wide) return wide
  for (const [key, value] of index.probeByGroup) {
    if (key.endsWith(`|${modelName}`)) return value
  }
  return undefined
}
