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
/**
 * How far from square an image may be and still be usable as a tab icon.
 *
 * A tab slot is square and the browser stretches whatever it is given to fill
 * it, so a wordmark (a 2:1 banner, say) arrives squashed. Only art that is
 * already square is used; anything else keeps the packaged icon.
 */
const FAVICON_ASPECT_TOLERANCE = 0.08

function replaceIconLink(href: string) {
  const existing =
    document.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]')
  if (existing.length === 1 && existing[0].href === href) return
  const link = document.createElement('link')
  link.rel = 'icon'
  link.href = href
  existing.forEach((l) => l.remove())
  document.head.appendChild(link)
}

export function applyFaviconToDom(url: string) {
  if (typeof document === 'undefined' || !url) return
  let next: string
  try {
    next = new URL(url, window.location.href).href
  } catch {
    // Malformed URL: leave whatever icon is already in place.
    return
  }
  // Whether the art is square is only knowable once it decodes, so the decision
  // is asynchronous. Until then the packaged icon stays, which is the safe
  // default: it is square by construction.
  const probe = new Image()
  probe.addEventListener(
    'load',
    () => {
      const { naturalWidth: width, naturalHeight: height } = probe
      if (width <= 0 || height <= 0) return
      const longest = Math.max(width, height)
      if (Math.abs(width - height) / longest > FAVICON_ASPECT_TOLERANCE) return
      replaceIconLink(next)
    },
    { once: true }
  )
  probe.src = next
}
