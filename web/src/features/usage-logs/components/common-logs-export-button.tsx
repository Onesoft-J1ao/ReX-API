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
import type { ColumnFiltersState } from '@tanstack/react-table'
import { Download, Loader2 } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { buildLogsCsv, csvFileName } from '../lib/csv'
import { fetchLogsByCategory } from '../lib/utils'
import type { LogCategory } from '../types'

/** Rows pulled per request while walking the filtered result set. */
const EXPORT_PAGE_SIZE = 1000

/**
 * Upper bound on exported rows. The walk pages until `total` is reached, so a
 * filter that matches half a million rows would otherwise issue hundreds of
 * requests and build a CSV in memory that no spreadsheet will open. When the
 * cap is hit the user is told the file is partial rather than silently getting
 * a truncated export.
 */
const EXPORT_MAX_ROWS = 20000

/**
 * Only the column metadata is needed, so the table is typed structurally —
 * the page's table is `Table<Record<string, unknown>>` and pinning the exact
 * log union here would only add a variance mismatch.
 */
interface ExportTable {
  getVisibleLeafColumns: () => Array<{
    id: string
    columnDef: { header?: unknown }
  }>
}

interface CommonLogsExportButtonProps {
  table: ExportTable
  logCategory: LogCategory
  isAdmin: boolean
  searchParams: Record<string, unknown>
  columnFilters: ColumnFiltersState
}

export function CommonLogsExportButton(props: CommonLogsExportButtonProps) {
  const { t } = useTranslation()
  const [exporting, setExporting] = useState(false)

  const handleExport = useCallback(async () => {
    setExporting(true)
    try {
      // Follow the visible table columns so the sheet matches what the user
      // is looking at, instead of a hardcoded field list that drifts.
      const columns = props.table
        .getVisibleLeafColumns()
        .filter((column) => column.id !== 'select' && column.id !== 'actions')
      if (columns.length === 0) {
        toast.error(t('Nothing to export'))
        return
      }

      const csvColumns = columns.map((column) => ({
        id: column.id,
        header: column.columnDef.header,
      }))

      const rows: Record<string, unknown>[] = []
      let page = 1
      let total = Number.POSITIVE_INFINITY
      for (;;) {
        const result = await fetchLogsByCategory({
          logCategory: props.logCategory,
          isAdmin: props.isAdmin,
          page,
          pageSize: EXPORT_PAGE_SIZE,
          searchParams: props.searchParams,
          columnFilters: props.columnFilters,
        })
        if (!result?.success || !result.data) {
          toast.error(result?.message || t('Failed to load logs'))
          return
        }
        const items = (result.data.items ?? []) as Record<string, unknown>[]
        total = result.data.total ?? items.length
        rows.push(...items)
        if (items.length < EXPORT_PAGE_SIZE || rows.length >= total) break
        if (rows.length >= EXPORT_MAX_ROWS) break
        page += 1
      }

      if (rows.length === 0) {
        toast.error(t('Nothing to export'))
        return
      }

      const truncated = rows.length < total
      const csv = buildLogsCsv(csvColumns, rows.slice(0, EXPORT_MAX_ROWS))
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = csvFileName(props.logCategory, new Date())
      document.body.append(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)

      if (truncated) {
        toast.warning(
          t('Exported the first {{count}} rows of {{total}}', {
            count: Math.min(rows.length, EXPORT_MAX_ROWS),
            total,
          })
        )
      } else {
        toast.success(t('Exported {{count}} rows', { count: rows.length }))
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Export failed'))
    } finally {
      setExporting(false)
    }
  }, [props, t])

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            onClick={() => void handleExport()}
            disabled={exporting}
            aria-label={t('Export as CSV')}
            className='text-muted-foreground hover:text-foreground size-7'
          />
        }
      >
        {exporting ? <Loader2 className='animate-spin' /> : <Download />}
      </TooltipTrigger>
      <TooltipContent>
        {exporting ? t('Exporting...') : t('Export as CSV')}
      </TooltipContent>
    </Tooltip>
  )
}
