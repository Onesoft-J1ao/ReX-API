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
import type * as Registry from './lobe-icon-registry'

// The registry is ~5 MB of SVG path data and stays out of the static graph so it
// never lands in the first paint. Both the renderer and the idle-time preload go
// through here so they share one promise: main.tsx calls preloadLobeIcons()
// while the browser is idle, and by the time a page renders a provider badge the
// renderer finds `loaded` already resolved and draws the icon synchronously.
export let loaded: typeof Registry | null = null
let pending: Promise<typeof Registry> | null = null

export function requestLobeIconRegistry(): Promise<typeof Registry> {
  pending = pending ?? import('./lobe-icon-registry').then((module) => {
    loaded = module
    return module
  })
  return pending
}

export function preloadLobeIcons(): void {
  void requestLobeIconRegistry()
}