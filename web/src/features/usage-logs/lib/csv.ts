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

/** A column the CSV mirrors from the on-screen table. */
export interface CsvColumn {
  id: string
  header?: unknown
}

/**
 * Quote a CSV field per RFC 4180. A field needs wrapping when it contains the
 * delimiter, a double quote, or a line break; embedded quotes are doubled.
 * Objects are JSON-encoded so nested log payloads survive the round trip
 * instead of collapsing to "[object Object]".
 */
export function csvField(value: unknown): string {
  if (value === null || value === undefined) return ''
  const text =
    typeof value === 'object' ? JSON.stringify(value) : String(value)
  if (/[",\r\n]/.test(text)) {
    return `"${text.replaceAll('"', '""')}"`
  }
  return text
}

/**
 * Header label for a column. Column headers are frequently React nodes, which
 * are useless in a spreadsheet, so only string headers are used verbatim and
 * everything else falls back to the column id.
 */
export function csvHeader(column: CsvColumn): string {
  return typeof column.header === 'string' ? column.header : column.id
}

/**
 * Build the CSV body. Rows are CRLF-joined per RFC 4180 and the result is
 * prefixed with a UTF-8 BOM, without which Excel decodes the file using the
 * local ANSI codepage and mangles Chinese model and group names.
 */
export function buildLogsCsv(
  columns: CsvColumn[],
  rows: Array<Record<string, unknown>>
): string {
  const lines = [columns.map((column) => csvField(csvHeader(column))).join(',')]
  for (const row of rows) {
    lines.push(columns.map((column) => csvField(row[column.id])).join(','))
  }
  return '﻿' + lines.join('\r\n')
}

/** Timestamped file name, e.g. `common-logs-20260913-2115.csv`. */
export function csvFileName(category: string, date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const stamp = [
    date.getFullYear(),
    pad(date.getMonth() + 1),
    pad(date.getDate()),
  ].join('')
  const time = [pad(date.getHours()), pad(date.getMinutes())].join('')
  return `${category}-logs-${stamp}-${time}.csv`
}
