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
 * Embedded-browser detection.
 *
 * The console is also opened inside Android WebView and iOS in-app browsers,
 * where a few browser APIs behave differently: `window.open` returns null
 * unless the host implements its own window handler, and `navigator.clipboard`
 * is absent outside a secure context. Those differences are not errors the
 * user caused, so the UI should explain them rather than report a generic
 * failure — which needs to know it is running in one.
 *
 * Kept pure so it can be tested without a DOM.
 */

export type WebViewSignals = {
  userAgent: string
}

/** Browser signals, or null when there is no navigator to read. */
export function readWebViewSignals(): WebViewSignals | null {
  if (typeof navigator === 'undefined') return null
  return { userAgent: navigator.userAgent ?? '' }
}

/**
 * Whether the page is running inside an app's embedded browser.
 *
 * Android WebView appends `; wv` to the platform token. iOS in-app browsers
 * (WKWebView) keep the `Mobile/... Safari/...` shape, so the usable difference
 * is that they are WebKit without a standalone Safari token. Both are
 * deliberately narrow: a false positive would tell a real browser user to
 * "open this in a real browser".
 */
export function isEmbeddedWebView(signals: WebViewSignals | null): boolean {
  if (!signals) return false
  const userAgent = signals.userAgent
  if (!userAgent) return false

  if (/;\s*wv\)/i.test(userAgent)) return true

  const isAppleMobile = /iPhone|iPad|iPod/i.test(userAgent)
  if (
    isAppleMobile &&
    /AppleWebKit/i.test(userAgent) &&
    !/Safari\//i.test(userAgent)
  ) {
    return true
  }

  return false
}

/** Whether the page is running inside an embedded browser, if navigator exists. */
export function inEmbeddedWebView(): boolean {
  return isEmbeddedWebView(readWebViewSignals())
}
