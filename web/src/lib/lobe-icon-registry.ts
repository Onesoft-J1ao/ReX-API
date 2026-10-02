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
import * as LobeIcons from '@lobehub/icons'
import type React from 'react'

// This module carries ~5 MB of SVG path data and is reached through one dynamic
// import (see ./lobe-icon). Keeping it out of the static graph is what holds it
// out of the first paint; the app preloads it during idle time so a page that
// renders a provider badge still gets a synchronous icon.

export const lobeIconRegistry = LobeIcons as unknown as Record<string, unknown>

type TocEntry = { id: string; param?: { hasColor?: boolean } }

export function lobeIconNames(): string[] {
  const toc = (LobeIcons as unknown as { toc?: TocEntry[] }).toc ?? []
  return toc.flatMap((icon) =>
    icon.param?.hasColor ? [icon.id, `${icon.id}.Color`] : [icon.id]
  )
}

export function resolveLobeIcon(
  baseKey: string
): Record<string, unknown> | undefined {
  return lobeIconRegistry[baseKey] as Record<string, unknown> | undefined
}

export type LobeIconComponent = React.ComponentType<Record<string, unknown>>
