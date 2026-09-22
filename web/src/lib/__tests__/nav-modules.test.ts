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

import {
  normalizeSidebarAdminLink,
  parseSidebarAdminLinks,
  parseSidebarAdminLinksFromStatus,
} from '../nav-modules'

describe('normalizeSidebarAdminLink', () => {
  it('keeps a titled absolute http(s) address', () => {
    expect(
      normalizeSidebarAdminLink({
        title: 'Kiro-Go',
        url: 'https://tools.example.com/admin',
      })
    ).toEqual({ title: 'Kiro-Go', url: 'https://tools.example.com/admin' })
  })

  it('trims the title and the address', () => {
    expect(
      normalizeSidebarAdminLink({
        title: '  Kiro-Go  ',
        url: '  https://tools.example.com/admin  ',
      })
    ).toEqual({ title: 'Kiro-Go', url: 'https://tools.example.com/admin' })
  })

  // The stored option is operator-editable text that ends up in an anchor's
  // href, so anything that is not a plain web address must not survive.
  it.each([
    ['javascript scheme', { title: 'x', url: 'javascript:alert(1)' }],
    ['data scheme', { title: 'x', url: 'data:text/html,<script></script>' }],
    ['file scheme', { title: 'x', url: 'file:///etc/passwd' }],
    ['relative address', { title: 'x', url: '/admin' }],
    ['scheme-relative address', { title: 'x', url: '//example.com/admin' }],
  ])('rejects a %s', (_name, raw) => {
    expect(normalizeSidebarAdminLink(raw)).toBeNull()
  })

  it.each([
    ['empty title', { title: '', url: 'https://example.com' }],
    ['blank title', { title: '   ', url: 'https://example.com' }],
    ['missing title', { url: 'https://example.com' }],
    ['empty address', { title: 'x', url: '' }],
    ['missing address', { title: 'x' }],
    ['string entry', 'https://example.com'],
    ['null entry', null],
    ['empty object', {}],
  ])('rejects %s', (_name, raw) => {
    expect(normalizeSidebarAdminLink(raw)).toBeNull()
  })
})

describe('parseSidebarAdminLinks', () => {
  it('keeps only the entries that survive validation', () => {
    const raw = JSON.stringify([
      { title: 'Kiro-Go', url: 'https://tools.example.com/admin' },
      { title: '', url: 'https://broken.example.com' },
      { title: 'Hostile', url: 'javascript:alert(1)' },
      { title: 'Moemail', url: 'https://mail.example.com' },
    ])

    expect(parseSidebarAdminLinks(raw)).toEqual([
      { title: 'Kiro-Go', url: 'https://tools.example.com/admin' },
      { title: 'Moemail', url: 'https://mail.example.com' },
    ])
  })

  it.each([
    ['an absent value', undefined],
    ['a null value', null],
    ['an empty string', ''],
    ['a blank string', '   '],
    ['unparsable text', 'not json'],
    ['a JSON object rather than an array', '{"title":"x","url":"https://e.com"}'],
    ['a JSON string rather than an array', '"https://example.com"'],
    ['an array of non-objects', '["https://example.com", 7, null]'],
  ])('returns nothing for %s', (_name, raw) => {
    expect(parseSidebarAdminLinks(raw)).toEqual([])
  })

  it('returns an empty list for an empty array', () => {
    expect(parseSidebarAdminLinks('[]')).toEqual([])
  })
})

describe('parseSidebarAdminLinksFromStatus', () => {
  it('reads the configured links off the status payload', () => {
    const status = {
      SidebarAdminLinks: JSON.stringify([
        { title: 'Kiro-Go', url: 'https://tools.example.com/admin' },
      ]),
    }

    expect(parseSidebarAdminLinksFromStatus(status)).toEqual([
      { title: 'Kiro-Go', url: 'https://tools.example.com/admin' },
    ])
  })

  // No default: a fallback address is the hardcoded link this configuration
  // exists to remove, so an unconfigured deployment shows nothing.
  it.each([
    ['a missing status', null],
    ['a status without the key', {}],
    ['a blank configured value', { SidebarAdminLinks: '' }],
  ])('returns nothing for %s', (_name, status) => {
    expect(parseSidebarAdminLinksFromStatus(status)).toEqual([])
  })
})
