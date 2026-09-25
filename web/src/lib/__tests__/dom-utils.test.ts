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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { applyFaviconToDom } from '../dom-utils'

/** The packaged icon the page ships with, square by construction. */
const PACKAGED_ICON = '/logo.png'

function currentIconHref(): string | null {
  const link = document.querySelector<HTMLLinkElement>('link[rel~="icon"]')
  return link ? new URL(link.href, window.location.href).pathname : null
}

/**
 * Stand in for the browser's Image: reports the given intrinsic size and fires
 * `load` on a later tick. A null size means the image never loads.
 */
function stubImage(width: number | null, height?: number) {
  class FakeImage {
    naturalWidth = width ?? 0
    naturalHeight = height ?? width ?? 0
    private listeners = new Map<string, Array<() => void>>()
    set src(_value: string) {
      if (width === null) return
      const fire = () => this.listeners.get('load')?.forEach((fn) => fn())
      // Async like a real decode, so the caller cannot depend on ordering.
      void Promise.resolve().then(fire)
    }
    addEventListener(type: string, callback: () => void) {
      const existing = this.listeners.get(type) ?? []
      existing.push(callback)
      this.listeners.set(type, existing)
    }
  }
  vi.stubGlobal('Image', FakeImage)
}

beforeEach(() => {
  document.head.innerHTML = `<link rel="icon" type="image/png" href="${PACKAGED_ICON}" />`
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('applyFaviconToDom', () => {
  it('uses a square logo as the tab icon', async () => {
    stubImage(180)

    applyFaviconToDom('https://cdn.example.com/square-logo.png')
    await vi.waitFor(() => {
      expect(currentIconHref()).toBe('/square-logo.png')
    })
  })

  // A tab slot is square. A wordmark is letterboxed when a canvas is available;
  // this environment cannot draw one, so the packaged icon stays.
  it('keeps the packaged icon when the logo is not square', async () => {
    stubImage(821, 430)

    applyFaviconToDom('https://cdn.example.com/wordmark.png')
    // Give the asynchronous decode a chance to run and be rejected.
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(currentIconHref()).toBe(PACKAGED_ICON)
  })

  it('tolerates a logo that is only slightly off square', async () => {
    stubImage(512, 500)

    applyFaviconToDom('https://cdn.example.com/nearly-square.png')
    await vi.waitFor(() => {
      expect(currentIconHref()).toBe('/nearly-square.png')
    })
  })

  it('keeps the packaged icon when the logo never loads', async () => {
    stubImage(null)

    applyFaviconToDom('https://cdn.example.com/broken.png')
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(currentIconHref()).toBe(PACKAGED_ICON)
  })

  it('ignores a malformed logo address', () => {
    stubImage(180)

    expect(() => applyFaviconToDom('http://[')).not.toThrow()
    expect(currentIconHref()).toBe(PACKAGED_ICON)
  })

  it('does nothing without an address', () => {
    stubImage(180)

    applyFaviconToDom('')

    expect(currentIconHref()).toBe(PACKAGED_ICON)
  })
})
