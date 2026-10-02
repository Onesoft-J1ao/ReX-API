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
import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { useCommonLogsColumns } from '../columns/common-logs-columns'

// The usage log table renders under `tableLayout: 'fixed'` so the colgroup widths
// decide the column geometry. A column without a declared size falls back to
// content sizing in that mode, which is what let the header drift out of line
// with the body on wide log tables.
describe('common log column sizing', () => {
  // A column declares its identity either with `id` or with `accessorKey`;
  // TanStack only derives `.id` once the column defs reach a table instance, so
  // the raw defs have to be matched on both.
  const keyOf = (column: { id?: string; accessorKey?: string }) =>
    column.id ?? (column.accessorKey as string | undefined)

  it('declares a width for every column', () => {
    const { result } = renderHook(() => useCommonLogsColumns(true, true))
    const unsized = result.current
      .filter((column) => keyOf(column) !== 'actions')
      .filter((column) => typeof column.size !== 'number')

    expect(unsized.map(keyOf)).toEqual([])
  })

  it('gives the content-hungry columns room for their values', () => {
    const { result } = renderHook(() => useCommonLogsColumns(true, true))
    const size = (id: string) =>
      result.current.find((column) => keyOf(column) === id)?.size

    expect(size('created_at')).toBeGreaterThanOrEqual(160)
    expect(size('model_name')).toBeGreaterThanOrEqual(200)
    expect(size('user')).toBeGreaterThanOrEqual(140)
    expect(size('channel')).toBeGreaterThanOrEqual(180)
  })

  it('keeps the detail column within its declared bounds', () => {
    const { result } = renderHook(() => useCommonLogsColumns(true, true))
    const content = result.current.find(
      (column) => keyOf(column) === 'content'
    )

    expect(typeof content?.size).toBe('number')
    if (typeof content?.maxSize === 'number') {
      expect(content.size).toBeLessThanOrEqual(content.maxSize)
    }
  })
})