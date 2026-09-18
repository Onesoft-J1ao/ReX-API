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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { handleServerError } from '@/lib/handle-server-error'

import { advanceRefund } from '../api'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import type { RefundAdvanceAction } from '../types'
import { useRefunds } from './refunds-provider'

// Runtime-selected copy per target state; every key is registered in
// web/src/i18n/static-keys.ts.
const ADVANCE_COPY: Record<
  RefundAdvanceAction,
  { title: string; description: string; confirm: string; destructive: boolean }
> = {
  approved: {
    title: 'Approve this refund request?',
    description:
      'A plan refund converts the remaining allowance into the wallet balance and ends the subscription.',
    confirm: 'Approve',
    destructive: false,
  },
  credited: {
    title: 'Mark this refund request as credited?',
    description:
      'Confirm that the payout has reached the user. This cannot be undone.',
    confirm: 'Mark as Credited',
    destructive: false,
  },
  rejected: {
    title: 'Reject this refund request?',
    description:
      'Any balance frozen by this request is returned to the user. This cannot be undone.',
    confirm: 'Reject',
    destructive: true,
  },
}

export function RefundsAdvanceDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, advanceAction, triggerRefresh } =
    useRefunds()
  const [isSubmitting, setIsSubmitting] = useState(false)

  const copy = advanceAction ? ADVANCE_COPY[advanceAction] : null

  const handleConfirm = async () => {
    if (!currentRow || !advanceAction) return

    setIsSubmitting(true)
    try {
      const result = await advanceRefund(currentRow.id, advanceAction)
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.ADVANCED))
        setOpen(null)
        triggerRefresh()
      } else {
        handleServerError(result, t(ERROR_MESSAGES.ADVANCE_FAILED))
      }
    } catch (error) {
      handleServerError(error, t(ERROR_MESSAGES.ADVANCE_FAILED))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <AlertDialog
      open={open === 'advance' && copy !== null}
      onOpenChange={(isOpen) => !isOpen && setOpen(null)}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{copy ? t(copy.title) : ''}</AlertDialogTitle>
          <AlertDialogDescription>
            {copy ? t(copy.description) : ''}
            {currentRow && (
              <>
                {' '}
                <span className='font-semibold'>
                  {t('User {{id}}', { id: currentRow.user_id })}
                </span>
              </>
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isSubmitting}>
            {t('Cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={handleConfirm}
            disabled={isSubmitting}
            variant={copy?.destructive ? 'destructive' : undefined}
          >
            {copy ? t(copy.confirm) : t('Confirm')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
