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
import type { Table } from '@tanstack/react-table'
import { Copy, Trash2, Loader2, Power, PowerOff } from 'lucide-react'
import { useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { handleServerError } from '@/lib/handle-server-error'

import { updateApiKeyStatus } from '../api'
import { API_KEY_STATUS, ERROR_MESSAGES } from '../constants'
import type { ApiKey } from '../types'
import { ApiKeysMultiDeleteDialog } from './api-keys-multi-delete-dialog'
import { useApiKeys } from './api-keys-provider'

type DataTableBulkActionsProps<TData> = {
  table: Table<TData>
}

export function DataTableBulkActions<TData>({
  table,
}: DataTableBulkActionsProps<TData>) {
  const { t } = useTranslation()
  const { resolveRealKeysBatch, triggerRefresh } = useApiKeys()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [isCopying, setIsCopying] = useState(false)
  const [isUpdatingStatus, setIsUpdatingStatus] = useState(false)
  const selectedRows = table.getFilteredSelectedRowModel().rows

  const handleBatchCopy = useCallback(async () => {
    if (selectedRows.length === 0) return

    setIsCopying(true)
    try {
      const ids = selectedRows.map((row) => (row.original as ApiKey).id)
      const keysMap = await resolveRealKeysBatch(ids)

      const lines: string[] = []
      for (const row of selectedRows) {
        const apiKey = row.original as ApiKey
        const realKey = keysMap[apiKey.id]
        if (realKey) {
          lines.push(`${apiKey.name}\t${realKey}`)
        }
      }

      if (lines.length > 0) {
        const ok = await copyToClipboard(lines.join('\n'))
        if (ok) {
          toast.success(t('Copied {{count}} key(s)', { count: lines.length }))
        } else {
          toast.error(t('Failed to copy keys'))
        }
      }
    } catch {
      toast.error(t('Failed to copy keys'))
    } finally {
      setIsCopying(false)
    }
  }, [selectedRows, resolveRealKeysBatch, t])

  // The backend only exposes a single-key status endpoint, so a bulk change is
  // one request per selected key. allSettled keeps a single failure from hiding
  // the keys that did change, and the partial result is reported honestly.
  const handleBatchStatus = useCallback(
    async (status: number) => {
      const ids = selectedRows.map((row) => (row.original as ApiKey).id)
      if (ids.length === 0) return

      setIsUpdatingStatus(true)
      try {
        const results = await Promise.allSettled(
          ids.map((id) => updateApiKeyStatus(id, status))
        )
        const failedCount = results.reduce((count, result) => {
          if (result.status === 'rejected') return count + 1
          return result.value?.success === false ? count + 1 : count
        }, 0)
        const succeededCount = ids.length - failedCount
        const isDisabling = status === API_KEY_STATUS.DISABLED

        if (failedCount === 0) {
          toast.success(
            isDisabling
              ? t('Disabled {{count}} API key(s)', { count: succeededCount })
              : t('Enabled {{count}} API key(s)', { count: succeededCount })
          )
        } else if (succeededCount === 0) {
          toast.error(
            t('Failed to update {{count}} API key(s)', { count: failedCount })
          )
        } else {
          toast.warning(
            t('Failed to update {{count}} API key(s)', { count: failedCount })
          )
        }

        if (failedCount === 0) table.resetRowSelection()
        triggerRefresh()
      } catch (error) {
        handleServerError(error, t(ERROR_MESSAGES.STATUS_UPDATE_FAILED))
      } finally {
        setIsUpdatingStatus(false)
      }
    },
    [selectedRows, t, table, triggerRefresh]
  )

  return (
    <>
      <BulkActionsToolbar table={table} entityName='API key'>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                onClick={() => handleBatchStatus(API_KEY_STATUS.DISABLED)}
                disabled={isUpdatingStatus}
                aria-label={t('Disable selected API keys')}
              />
            }
          >
            {isUpdatingStatus ? (
              <Loader2 className='size-4 animate-spin' />
            ) : (
              <PowerOff className='size-4' />
            )}
            <span className='sr-only'>{t('Disable selected API keys')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Disable selected API keys')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                onClick={() => handleBatchStatus(API_KEY_STATUS.ENABLED)}
                disabled={isUpdatingStatus}
                aria-label={t('Enable selected API keys')}
              />
            }
          >
            <Power className='size-4' />
            <span className='sr-only'>{t('Enable selected API keys')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Enable selected API keys')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                onClick={handleBatchCopy}
                disabled={isCopying}
                aria-label={t('Copy selected keys')}
              />
            }
          >
            {isCopying ? (
              <Loader2 className='size-4 animate-spin' />
            ) : (
              <Copy className='size-4' />
            )}
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Copy selected keys')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                onClick={() => setShowDeleteConfirm(true)}
                className='size-8'
                aria-label={t('Delete selected API keys')}
              />
            }
          >
            <Trash2 />
            <span className='sr-only'>{t('Delete selected API keys')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Delete selected API keys')}</p>
          </TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      <ApiKeysMultiDeleteDialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        table={table}
      />
    </>
  )
}
