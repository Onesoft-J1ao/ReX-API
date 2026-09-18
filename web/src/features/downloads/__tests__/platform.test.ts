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
  detectClientPlatform,
  detectClientArch,
  isRecommendedBuild,
} from '../lib/platform'
import type { ClientPlatform, DetectionSignals } from '../types'

const WINDOWS_X64_UA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36'
const WINDOWS_ARM64_UA =
  'Mozilla/5.0 (Windows NT 10.0; ARM64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36'
const MACOS_UA =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15'
const IOS_UA =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'

type PlatformCase = {
  name: string
  signals: DetectionSignals
  expected: ClientPlatform
}

describe('detectClientPlatform', () => {
  test.each<PlatformCase>([
    {
      name: 'reads the client hint platform when the browser provides it',
      signals: { clientHintPlatform: 'Windows', userAgent: MACOS_UA },
      expected: 'windows',
    },
    {
      name: 'maps an iPhone User-Agent to unknown so no desktop build is highlighted',
      signals: { userAgent: IOS_UA, platform: 'iPhone' },
      expected: 'unknown',
    },
    {
      name: 'maps an iPhone User-Agent to unknown even when platform claims MacIntel',
      signals: { userAgent: IOS_UA, platform: 'MacIntel' },
      expected: 'unknown',
    },
    {
      name: 'maps Android to unknown rather than to desktop Linux',
      signals: { userAgent: 'Mozilla/5.0 (Linux; Android 15) Mobile Safari/537.36' },
      expected: 'unknown',
    },
    {
      name: 'maps a ChromeOS client hint to unknown',
      signals: { clientHintPlatform: 'Chrome OS' },
      expected: 'unknown',
    },
    {
      name: 'detects macOS from the User-Agent when no client hint is sent',
      signals: { userAgent: MACOS_UA },
      expected: 'macos',
    },
    {
      name: 'detects Linux from the User-Agent',
      signals: { userAgent: 'Mozilla/5.0 (X11; Ubuntu; Linux x86_64) Firefox/131.0' },
      expected: 'linux',
    },
    {
      name: 'falls back to navigator.platform when the User-Agent is unreadable',
      signals: { userAgent: '', platform: 'Win32' },
      expected: 'windows',
    },
    {
      name: 'reports unknown when nothing identifies the platform',
      signals: { userAgent: 'SomeUnusualAgent/1.0', platform: '' },
      expected: 'unknown',
    },
  ])('$name', ({ signals, expected }) => {
    expect(detectClientPlatform(signals)).toBe(expected)
  })
})

describe('detectClientArch', () => {
  test('reads arm64 from the high-entropy client hint', () => {
    expect(
      detectClientArch({
        clientHintArchitecture: 'arm',
        userAgent: WINDOWS_X64_UA,
      })
    ).toBe('arm64')
  })

  test('reads arm64 from a Windows ARM64 User-Agent', () => {
    expect(detectClientArch({ userAgent: WINDOWS_ARM64_UA })).toBe('arm64')
  })

  test('reads x64 from a 64-bit Windows User-Agent', () => {
    expect(detectClientArch({ userAgent: WINDOWS_X64_UA })).toBe('x64')
  })

  test('keeps the architecture unknown for MacIntel because Safari reports it on Apple Silicon', () => {
    expect(detectClientArch({ userAgent: MACOS_UA, platform: 'MacIntel' })).toBe(
      'unknown'
    )
  })

  test('reads arm64 from an Apple Silicon platform token', () => {
    expect(detectClientArch({ platform: 'MacARM' })).toBe('arm64')
  })

  test('reads arm64 from a Linux ARM platform token', () => {
    expect(detectClientArch({ platform: 'Linux armv8l' })).toBe('arm64')
  })
})

describe('isRecommendedBuild', () => {
  test('recommends the universal macOS build for any macOS architecture', () => {
    expect(
      isRecommendedBuild(
        { platform: 'macos', arch: 'universal' },
        { platform: 'macos', arch: 'unknown' }
      )
    ).toBe(true)
  })

  test('recommends the platform build whose architecture matches the detected CPU', () => {
    const windowsArm = { platform: 'windows' as const, arch: 'arm64' as const }

    expect(
      isRecommendedBuild(windowsArm, { platform: 'windows', arch: 'arm64' })
    ).toBe(true)
    expect(
      isRecommendedBuild(windowsArm, { platform: 'windows', arch: 'x64' })
    ).toBe(false)
  })

  test('falls back to x64 for Windows when the browser hides the CPU', () => {
    expect(
      isRecommendedBuild(
        { platform: 'windows', arch: 'x64' },
        { platform: 'windows', arch: 'unknown' }
      )
    ).toBe(true)
  })

  test('recommends nothing for another platform or an unknown platform', () => {
    const windows = { platform: 'windows' as const, arch: 'x64' as const }

    expect(
      isRecommendedBuild(windows, { platform: 'macos', arch: 'x64' })
    ).toBe(false)
    expect(
      isRecommendedBuild(windows, { platform: 'unknown', arch: 'x64' })
    ).toBe(false)
  })
})
