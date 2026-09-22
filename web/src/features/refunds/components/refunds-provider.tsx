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
import React, { useState } from 'react'

import useDialogState from '@/hooks/use-dialog'

import type { RefundAdvanceAction, RefundRequest, RefundsDialogType } from '../types'

type RefundsContextType = {
  open: RefundsDialogType | null
  setOpen: (str: RefundsDialogType | null) => void
  currentRow: RefundRequest | null
  setCurrentRow: React.Dispatch<React.SetStateAction<RefundRequest | null>>
  advanceAction: RefundAdvanceAction | null
  setAdvanceAction: React.Dispatch<React.SetStateAction<RefundAdvanceAction | null>>
  refreshTrigger: number
  triggerRefresh: () => void
}

const RefundsContext = React.createContext<RefundsContextType | null>(null)

export function RefundsProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useDialogState<RefundsDialogType>(null)
  const [currentRow, setCurrentRow] = useState<RefundRequest | null>(null)
  const [advanceAction, setAdvanceAction] = useState<RefundAdvanceAction | null>(
    null
  )
  const [refreshTrigger, setRefreshTrigger] = useState(0)

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  return (
    <RefundsContext
      value={{
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        advanceAction,
        setAdvanceAction,
        refreshTrigger,
        triggerRefresh,
      }}
    >
      {children}
    </RefundsContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useRefunds = () => {
  const refundsContext = React.useContext(RefundsContext)

  if (!refundsContext) {
    throw new Error('useRefunds has to be used within <RefundsProvider>')
  }

  return refundsContext
}
