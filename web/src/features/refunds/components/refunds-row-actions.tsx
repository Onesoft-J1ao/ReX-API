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
import type { Row } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { DataTableRowActionMenu } from '@/components/data-table/core/row-action-menu'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'

import { getRefundAdvanceActions } from '../constants'
import { refundRequestSchema } from '../types'
import { useRefunds } from './refunds-provider'

interface RefundsRowActionsProps<TData> {
  row: Row<TData>
}

export function RefundsRowActions<TData>(props: RefundsRowActionsProps<TData>) {
  const { t } = useTranslation()
  const refund = refundRequestSchema.parse(props.row.original)
  const { setOpen, setCurrentRow, setAdvanceAction } = useRefunds()
  const actions = getRefundAdvanceActions(refund.status)

  if (actions.length === 0) {
    return <span className='text-muted-foreground text-sm'>-</span>
  }

  return (
    <div className='-ml-1.5 flex items-center gap-1'>
      <DataTableRowActionMenu ariaLabel={t('Open menu')} modal={false}>
        {actions.map((config, index) => (
          <div key={config.action}>
            {index > 0 && config.destructive && <DropdownMenuSeparator />}
            <DropdownMenuItem
              onClick={() => {
                setCurrentRow(refund)
                setAdvanceAction(config.action)
                setOpen('advance')
              }}
              className={
                config.destructive
                  ? 'text-destructive focus:text-destructive'
                  : undefined
              }
            >
              {t(config.labelKey)}
            </DropdownMenuItem>
          </div>
        ))}
      </DataTableRowActionMenu>
    </div>
  )
}
