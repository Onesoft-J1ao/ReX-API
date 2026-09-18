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
import { Ban, Download, Power, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { formatQuota, formatTimestamp } from '@/lib/format'

import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'

import { manageUser } from '../api'
import { USER_ROLE, USER_STATUS, isUserDeleted } from '../constants'
import { getUserActionMessage } from '../lib'
import type { User } from '../types'
import { useUsers } from './users-provider'

const CSV_HEADERS = [
  'id',
  'username',
  'display_name',
  'email',
  'github_id',
  'oidc_id',
  'wechat_id',
  'telegram_id',
  'linux_do_id',
  'group',
  'role',
  'status',
  'quota',
  'used_quota',
  'request_count',
  'aff_code',
  'aff_count',
  'aff_history_quota',
  'inviter_id',
  'remark',
  'created_at',
  'last_login_at',
] as const

const ROLE_LABEL: Record<number, string> = {
  [USER_ROLE.USER]: 'user',
  [USER_ROLE.ADMIN]: 'admin',
  [USER_ROLE.ROOT]: 'root',
}

const STATUS_LABEL: Record<number, string> = {
  [USER_STATUS.ENABLED]: 'enabled',
  [USER_STATUS.DISABLED]: 'disabled',
  [USER_STATUS.DELETED]: 'deleted',
}

function csvCell(value: unknown): string {
  const text = value === null || value === undefined ? '' : String(value)
  if (/[",\n\r]/.test(text)) {
    return `"${text.replaceAll('"', '""')}"`
  }
  return text
}

function exportUsersCsv(users: User[]): void {
  const rows = users.map((user) =>
    CSV_HEADERS.map((header) => {
      const value = (user as Record<string, unknown>)[header]
      if (header === 'role' && typeof value === 'number') {
        return ROLE_LABEL[value] ?? String(value)
      }
      if (header === 'status' && typeof value === 'number') {
        return STATUS_LABEL[value] ?? String(value)
      }
      if (header === 'created_at' || header === 'last_login_at') {
        const ts = value as number | undefined
        return ts ? formatTimestamp(ts) : ''
      }
      if (header === 'quota' || header === 'used_quota') {
        return typeof value === 'number' ? formatQuota(value) : csvCell(value)
      }
      return csvCell(value)
    }).join(',')
  )
  const csv = [CSV_HEADERS.join(','), ...rows].join('\r\n')
  const blob = new Blob([`﻿${csv}`], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `users-${new Date().toISOString().slice(0, 10)}.csv`
  link.click()
  URL.revokeObjectURL(url)
}

function confirmTextFor(
  action: 'disable' | 'enable' | 'delete',
  t: (key: string) => string
): string {
  if (action === 'delete') return t('Delete')
  if (action === 'disable') return t('Ban')
  return t('Enable')
}

interface DataTableBulkActionsProps {
  table: Table<User>
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUsers()
  const [confirmAction, setConfirmAction] = useState<
    'disable' | 'enable' | 'delete' | null
  >(null)
  const [running, setRunning] = useState(false)

  const selectedRows = table.getFilteredSelectedRowModel().rows
  const selectedUsers = useMemo(
    () => selectedRows.map((row) => row.original),
    [selectedRows]
  )
  // Root accounts are never bulk-managed: the backend rejects it and the row
  // menu hides destructive actions for them, so the bulk toolbar matches.
  const actionableUsers = useMemo(
    () => selectedUsers.filter((user) => user.role < USER_ROLE.ROOT),
    [selectedUsers]
  )
  const actionableCount = actionableUsers.length
  const skippedRoots = selectedUsers.length - actionableCount
  const deletableUsers = useMemo(
    () =>
      actionableUsers.filter(
        (user) => !isUserDeleted(user) && user.status !== USER_STATUS.DELETED
      ),
    [actionableUsers]
  )

  const runManage = async (
    users: User[],
    action: 'disable' | 'enable'
  ): Promise<void> => {
    let ok = 0
    let failed = 0
    for (const user of users) {
      try {
        const result = await manageUser(user.id, action)
        if (result.success) {
          ok += 1
        } else {
          failed += 1
        }
      } catch {
        failed += 1
      }
    }
    if (failed > 0) {
      toast.warning(
        t('{{ok}} succeeded, {{failed}} failed', { ok, failed })
      )
    } else {
      toast.success(t(getUserActionMessage(action)))
    }
    table.resetRowSelection()
    triggerRefresh()
  }

  const runDelete = async (users: User[]): Promise<void> => {
    let ok = 0
    let failed = 0
    for (const user of users) {
      try {
        const result = await manageUser(user.id, 'delete')
        if (result.success) {
          ok += 1
        } else {
          failed += 1
        }
      } catch {
        failed += 1
      }
    }
    if (failed > 0) {
      toast.warning(t('{{ok}} succeeded, {{failed}} failed', { ok, failed }))
    } else {
      toast.success(t(getUserActionMessage('delete')))
    }
    table.resetRowSelection()
    triggerRefresh()
  }

  const handleConfirm = async () => {
    if (!confirmAction) return
    setRunning(true)
    try {
      if (confirmAction === 'delete') {
        await runDelete(deletableUsers)
      } else {
        await runManage(actionableUsers, confirmAction)
      }
    } finally {
      setRunning(false)
      setConfirmAction(null)
    }
  }

  let confirmTitle: string
  let confirmDesc: string
  if (confirmAction === 'delete') {
    confirmTitle = t('Delete {{count}} users?', { count: deletableUsers.length })
    confirmDesc = t(
      'This will permanently delete {{count}} selected users. This action cannot be undone.',
      { count: deletableUsers.length }
    )
  } else if (confirmAction === 'disable') {
    confirmTitle = t('Disable {{count}} users?', {
      count: actionableUsers.length,
    })
    confirmDesc = t(
      'Selected users will be banned from signing in and consuming quota.'
    )
  } else {
    confirmTitle = t('Enable {{count}} users?', {
      count: actionableUsers.length,
    })
    confirmDesc = t('Selected users will be able to sign in again.')
  }

  return (
    <>
      <BulkActionsToolbar table={table} entityName='user'>
        <Button
          variant='outline'
          size='sm'
          className='h-8 gap-1.5'
          disabled={actionableCount === 0}
          onClick={() => setConfirmAction('disable')}
        >
          <Ban className='size-3.5' />
          {t('Ban')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          className='h-8 gap-1.5'
          disabled={actionableCount === 0}
          onClick={() => setConfirmAction('enable')}
        >
          <Power className='size-3.5' />
          {t('Enable')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          className='text-destructive hover:text-destructive h-8 gap-1.5'
          disabled={deletableUsers.length === 0}
          onClick={() => setConfirmAction('delete')}
        >
          <Trash2 className='size-3.5' />
          {t('Delete')}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button variant='outline' size='sm' className='h-8 gap-1.5'>
                <Download className='size-3.5' />
                {t('Export')}
              </Button>
            }
          />
          <DropdownMenuContent align='start'>
            <DropdownMenuItem onClick={() => exportUsersCsv(selectedUsers)}>
              {t('Export selected ({{count}})', {
                count: selectedUsers.length,
              })}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => exportUsersCsv(table.getFilteredRowModel().rows.map((row) => row.original))}
            >
              {t('Export all filtered ({{count}})', {
                count: table.getFilteredRowModel().rows.length,
              })}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </BulkActionsToolbar>
      {skippedRoots > 0 && (
        <p className='text-muted-foreground mt-1 text-center text-xs'>
          {t('{{count}} root account(s) skipped for bulk actions', {
            count: skippedRoots,
          })}
        </p>
      )}
      <ConfirmDialog
        open={confirmAction !== null}
        onOpenChange={(open) => !open && setConfirmAction(null)}
        title={confirmTitle}
        desc={confirmDesc}
        confirmText={confirmAction ? confirmTextFor(confirmAction, t) : ''}        destructive={confirmAction === 'delete' || confirmAction === 'disable'}
        isLoading={running}
        handleConfirm={handleConfirm}
      />
    </>
  )
}
