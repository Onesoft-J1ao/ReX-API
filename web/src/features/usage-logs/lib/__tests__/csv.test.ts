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

import { buildLogsCsv, csvField, csvFileName, csvHeader } from '../csv'

const BOM = '﻿'

describe('csvField', () => {
  it('passes plain values through unquoted', () => {
    expect(csvField('gpt-5.5')).toBe('gpt-5.5')
    expect(csvField(42)).toBe('42')
    expect(csvField(false)).toBe('false')
  })

  it('renders missing values as an empty field', () => {
    expect(csvField(null)).toBe('')
    expect(csvField(undefined)).toBe('')
  })

  it('quotes fields containing the delimiter, quotes, or line breaks', () => {
    expect(csvField('a,b')).toBe('"a,b"')
    expect(csvField('say "hi"')).toBe('"say ""hi"""')
    expect(csvField('line1\nline2')).toBe('"line1\nline2"')
    expect(csvField('carriage\rreturn')).toBe('"carriage\rreturn"')
  })

  it('serialises nested payloads instead of collapsing them', () => {
    expect(csvField({ model: 'glm-5.3' })).toBe('"{""model"":""glm-5.3""}"')
  })
})

describe('csvHeader', () => {
  it('uses a string header verbatim', () => {
    expect(csvHeader({ id: 'model_name', header: 'Model' })).toBe('Model')
  })

  it('falls back to the column id for non-string headers', () => {
    // Table headers are usually React nodes, which are useless in a sheet.
    expect(csvHeader({ id: 'quota', header: {} })).toBe('quota')
    expect(csvHeader({ id: 'quota' })).toBe('quota')
  })
})

describe('buildLogsCsv', () => {
  it('prefixes a UTF-8 BOM so Excel keeps non-ASCII intact', () => {
    const csv = buildLogsCsv([{ id: 'model_name' }], [])
    expect(csv.startsWith(BOM)).toBe(true)
    expect(csv.slice(BOM.length)).toBe('model_name')
  })

  it('joins records with CRLF', () => {
    const csv = buildLogsCsv(
      [{ id: 'model_name' }, { id: 'quota' }],
      [
        { model_name: 'glm-5.3', quota: 1200 },
        { model_name: 'kimi-k3', quota: 0 },
      ]
    )
    expect(csv.slice(BOM.length)).toBe(
      'model_name,quota\r\nglm-5.3,1200\r\nkimi-k3,0'
    )
  })

  it('projects only the requested columns and blanks missing fields', () => {
    const csv = buildLogsCsv(
      [{ id: 'model_name' }, { id: 'username' }],
      [{ model_name: 'glm-5.3' }]
    )
    expect(csv.slice(BOM.length)).toBe('model_name,username\r\nglm-5.3,')
  })

  it('escapes a value that would otherwise break column alignment', () => {
    const csv = buildLogsCsv(
      [{ id: 'token_name' }],
      [{ token_name: 'prod, "main"' }]
    )
    expect(csv.slice(BOM.length)).toBe(
      'token_name\r\n"prod, ""main"""'
    )
  })
})

describe('csvFileName', () => {
  it('stamps the category and reads as a sortable date', () => {
    expect(csvFileName('common', new Date(2026, 8, 13, 21, 5))).toBe(
      'common-logs-20260913-2105.csv'
    )
  })
})
