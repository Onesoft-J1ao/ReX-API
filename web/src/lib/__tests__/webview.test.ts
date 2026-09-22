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
import { describe, expect, it } from 'vitest'

import { isEmbeddedWebView } from '../webview'

const ANDROID_WEBVIEW =
  'Mozilla/5.0 (Linux; Android 13; Pixel 7 Build/TQ3A.230805.001; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/119.0.0.0 Mobile Safari/537.36'
const ANDROID_CHROME =
  'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Mobile Safari/537.36'
const IOS_SAFARI =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1'
const IOS_IN_APP =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148'
const DESKTOP_CHROME =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36'
// The desktop shell reports Electron, but it does implement window.open, so it
// must not be told to "open this in a real browser".
const ELECTRON =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) new-api/1.0.0 Chrome/120.0.0.0 Electron/28.0.0 Safari/537.36'

describe('isEmbeddedWebView', () => {
  it.each([
    ['an Android WebView', ANDROID_WEBVIEW],
    ['an iOS in-app browser', IOS_IN_APP],
  ])('reports %s as embedded', (_name, userAgent) => {
    expect(isEmbeddedWebView({ userAgent })).toBe(true)
  })

  it.each([
    ['Android Chrome', ANDROID_CHROME],
    ['iOS Safari', IOS_SAFARI],
    ['desktop Chrome', DESKTOP_CHROME],
    ['the Electron shell', ELECTRON],
  ])('reports %s as a full browser', (_name, userAgent) => {
    expect(isEmbeddedWebView({ userAgent })).toBe(false)
  })

  it.each([
    ['no signals', null],
    ['an empty user agent', { userAgent: '' }],
    ['an unrelated user agent', { userAgent: 'curl/8.4.0' }],
  ])('reports %s as a full browser', (_name, signals) => {
    expect(isEmbeddedWebView(signals)).toBe(false)
  })
})
