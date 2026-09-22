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
import { describe, expect, test } from 'vitest'

import {
  buildCdnUrl,
  CC_SWITCH_FALLBACK_RELEASE,
  CC_SWITCH_MANIFEST_URL,
  CC_SWITCH_RELEASES_API,
  findCcSwitchAsset,
  parseCcSwitchGithubRelease,
  parseCcSwitchManifest,
  resolveCcSwitchRelease,
  resolveManifestUrl,
  type CcSwitchRelease,
} from '../lib/cc-switch'

const VERSION = '3.21.0'
const WINDOWS_FILE = `CC-Switch-v${VERSION}-Windows.msi`
const MACOS_TARBALL = `CC-Switch-v${VERSION}-macOS.tar.gz`

const MANIFEST_BODY = {
  version: VERSION,
  platforms: {
    'darwin-aarch64': {
      url: `https://dl.ccswitch.io/v${VERSION}/${MACOS_TARBALL}`,
    },
    // The manifest repeats the same universal artifact per macOS CPU.
    'darwin-x86_64': {
      url: `https://dl.ccswitch.io/v${VERSION}/${MACOS_TARBALL}`,
    },
    'windows-x86_64': {
      url: `https://dl.ccswitch.io/v${VERSION}/${WINDOWS_FILE}`,
    },
    'windows-aarch64': { url: 42 },
  },
}

const GITHUB_BODY = {
  tag_name: `v${VERSION}`,
  assets: [
    {
      name: WINDOWS_FILE,
      browser_download_url: `https://github.com/farion1231/cc-switch/releases/download/v${VERSION}/${WINDOWS_FILE}`,
    },
    {
      name: `${WINDOWS_FILE}.sig`,
      browser_download_url: `https://github.com/farion1231/cc-switch/releases/download/v${VERSION}/${WINDOWS_FILE}.sig`,
    },
    {
      name: `CC-Switch-v${VERSION}-Windows-arm64.msi`,
      browser_download_url: `https://github.com/farion1231/cc-switch/releases/download/v${VERSION}/CC-Switch-v${VERSION}-Windows-arm64.msi`,
    },
  ],
}

type FetchHandler = () => Promise<Response>

function jsonResponse(body: unknown, ok = true): Response {
  return {
    ok,
    json: async () => body,
    text: async () => JSON.stringify(body),
  } as unknown as Response
}

function fetchStub(handlers: Record<string, FetchHandler>): typeof fetch {
  return ((input: RequestInfo | URL) => {
    const url = String(input)
    const handler = handlers[url]
    if (!handler) {
      return Promise.reject(new Error(`unexpected request: ${url}`))
    }
    return handler()
  }) as typeof fetch
}

/**
 * The parsers return `null` for payloads they cannot read, so a test that
 * asserts on parsed fields must first pin the contract that this payload is
 * one they can read.
 */
function requireRelease(release: CcSwitchRelease | null): CcSwitchRelease {
  if (!release) throw new Error('expected the release payload to parse')
  return release
}

describe('parseCcSwitchManifest', () => {
  test('reads the version and de-duplicates the repeated universal macOS artifact', () => {
    const release = parseCcSwitchManifest(MANIFEST_BODY)

    expect(release?.version).toBe(VERSION)
    expect(release?.source).toBe('manifest')
    expect(release?.assets.map((asset) => asset.fileName)).toEqual([
      MACOS_TARBALL,
      WINDOWS_FILE,
    ])
  })

  test('keeps the GitHub release asset as the mirror for each manifest URL', () => {
    const release = requireRelease(parseCcSwitchManifest(MANIFEST_BODY))

    expect(findCcSwitchAsset(release, '-Windows.msi')?.mirrorUrl).toBe(
      `https://github.com/farion1231/cc-switch/releases/download/v${VERSION}/${WINDOWS_FILE}`
    )
  })

  test('rejects a payload without a usable version or platform map', () => {
    expect(parseCcSwitchManifest(null)).toBeNull()
    expect(parseCcSwitchManifest('nope')).toBeNull()
    expect(parseCcSwitchManifest({ platforms: {} })).toBeNull()
    expect(parseCcSwitchManifest({ version: '  ' })).toBeNull()
    expect(
      parseCcSwitchManifest({ version: VERSION, platforms: { a: { url: 1 } } })
    ).toBeNull()
  })
})

describe('parseCcSwitchGithubRelease', () => {
  test('builds the official CDN URL from the tag and mirrors it on the GitHub asset', () => {
    const release = requireRelease(parseCcSwitchGithubRelease(GITHUB_BODY))
    const windows = findCcSwitchAsset(release, '-Windows.msi')

    expect(release.version).toBe(VERSION)
    expect(release.source).toBe('github')
    expect(windows?.fileName).toBe(WINDOWS_FILE)
    expect(windows?.url).toBe(buildCdnUrl(VERSION, WINDOWS_FILE))
    expect(windows?.mirrorUrl).toBe(
      `https://github.com/farion1231/cc-switch/releases/download/v${VERSION}/${WINDOWS_FILE}`
    )
  })

  test('drops signature sidecars because they are not installers', () => {
    const release = parseCcSwitchGithubRelease(GITHUB_BODY)

    expect(release?.assets.some((asset) => asset.fileName.endsWith('.sig'))).toBe(
      false
    )
  })

  test('rejects a payload without assets', () => {
    expect(parseCcSwitchGithubRelease({ tag_name: 'v1.0.0' })).toBeNull()
    expect(parseCcSwitchGithubRelease({ tag_name: '', assets: [] })).toBeNull()
    expect(parseCcSwitchGithubRelease({ assets: [] })).toBeNull()
  })
})

describe('findCcSwitchAsset', () => {
  test('does not let the ARM64 installer answer for the x64 suffix', () => {
    const release = requireRelease(parseCcSwitchGithubRelease(GITHUB_BODY))

    expect(findCcSwitchAsset(release, '-Windows.msi')?.fileName).toBe(
      WINDOWS_FILE
    )
    expect(
      findCcSwitchAsset(release, '-Windows-arm64.msi')?.fileName
    ).toBe(`CC-Switch-v${VERSION}-Windows-arm64.msi`)
  })
})

describe('resolveCcSwitchRelease', () => {
  test('prefers the official manifest when a reachable manifest URL is configured', async () => {
    const release = await resolveCcSwitchRelease({
      manifestUrl: 'https://proxy.example/cc-switch/latest.json',
      fetchImpl: fetchStub({
        'https://proxy.example/cc-switch/latest.json': async () =>
          jsonResponse(MANIFEST_BODY),
        [CC_SWITCH_RELEASES_API]: async () => jsonResponse(GITHUB_BODY),
      }),
    })

    expect(release.source).toBe('manifest')
    expect(release.version).toBe(VERSION)
  })

  test('uses the GitHub release API when no manifest URL is configured', async () => {
    const release = await resolveCcSwitchRelease({
      manifestUrl: null,
      fetchImpl: fetchStub({
        [CC_SWITCH_RELEASES_API]: async () => jsonResponse(GITHUB_BODY),
      }),
    })

    expect(release.source).toBe('github')
    expect(release.version).toBe(VERSION)
  })

  test('falls through to GitHub when the manifest responds with an unusable body', async () => {
    const release = await resolveCcSwitchRelease({
      manifestUrl: 'https://proxy.example/cc-switch/latest.json',
      fetchImpl: fetchStub({
        'https://proxy.example/cc-switch/latest.json': async () =>
          jsonResponse({ unexpected: true }),
        [CC_SWITCH_RELEASES_API]: async () => jsonResponse(GITHUB_BODY),
      }),
    })

    expect(release.source).toBe('github')
  })

  test('reports the pinned release when the GitHub API rejects the request', async () => {
    const release = await resolveCcSwitchRelease({
      manifestUrl: null,
      fetchImpl: fetchStub({
        [CC_SWITCH_RELEASES_API]: async () =>
          jsonResponse({ message: 'rate limited' }, false),
      }),
    })

    expect(release).toBe(CC_SWITCH_FALLBACK_RELEASE)
  })

  test('reports the pinned release when the network request fails outright', async () => {
    const release = await resolveCcSwitchRelease({
      manifestUrl: 'https://proxy.example/cc-switch/latest.json',
      fetchImpl: fetchStub({
        'https://proxy.example/cc-switch/latest.json': () =>
          Promise.reject(new Error('offline')),
        [CC_SWITCH_RELEASES_API]: () => Promise.reject(new Error('offline')),
      }),
    })

    expect(release.source).toBe('fallback')
  })

  test('pins a Windows x64 URL on the official CDN that the page can always offer', () => {
    const windows = findCcSwitchAsset(CC_SWITCH_FALLBACK_RELEASE, '-Windows.msi')

    expect(windows?.url).toBe(
      `https://dl.ccswitch.io/v${CC_SWITCH_FALLBACK_RELEASE.version}/CC-Switch-v${CC_SWITCH_FALLBACK_RELEASE.version}-Windows.msi`
    )
  })
})

describe('resolveManifestUrl', () => {
  test('skips the official manifest when it is cross-origin, because the CDN sends no CORS header', () => {
    expect(resolveManifestUrl()).not.toBe(CC_SWITCH_MANIFEST_URL)
  })
})
