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
import type React from 'react'

/**
 * Opacity steps, in percent, for the surfaces layered over a background image.
 * `styles/index.css` consumes these through `[data-console-background='image']`
 * to tint cards, lists and sidebars with `color-mix`, so every layout that
 * renders an image backdrop shares one transparency model.
 */
export function consoleSurfaceStyles(
  backgroundImage: string,
  backgroundBlurOpacity: number | undefined,
  /** Per-browser override for the surface density; null keeps the configured value. */
  opacityOverride?: number | null
): React.CSSProperties | undefined {
  if (!backgroundImage) return undefined
  const configured = Math.min(100, Math.max(0, backgroundBlurOpacity ?? 40))
  const blurOpacity =
    opacityOverride == null
      ? configured
      : Math.min(100, Math.max(0, opacityOverride))
  return {
    '--console-surface-opacity': `${blurOpacity}%`,
    // Outer containers stay quite sheer so the photo reads through them.
    '--console-card-opacity': `${Math.min(60, Math.round(blurOpacity * 0.85))}%`,
    // A card nested inside another is denser, so the two levels stay
    // distinguishable instead of flattening into one translucent sheet.
    '--console-inner-card-opacity': `${Math.min(95, blurOpacity + 45)}%`,
    '--console-sidebar-opacity': `${Math.min(80, blurOpacity + 10)}%`,
    '--console-list-opacity': `${Math.min(75, blurOpacity + 20)}%`,
  } as React.CSSProperties
}
