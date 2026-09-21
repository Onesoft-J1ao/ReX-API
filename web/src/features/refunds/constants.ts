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
import type { TFunction } from 'i18next'

import type { StatusBadgeProps } from '@/components/status-badge'

// ============================================================================
// Refund Kind Configuration
// ============================================================================

export const REFUND_KIND = {
  PLAN: 'plan',
  CASH: 'cash',
} as const

// labelKey values are i18n keys; use t(config.labelKey) in components
export const REFUND_KINDS: Record<string, string> = {
  [REFUND_KIND.PLAN]: 'Plan Refund',
  [REFUND_KIND.CASH]: 'Cash Refund',
}

// ============================================================================
// Refund Status Configuration
// ============================================================================

export const REFUND_STATUS = {
  PENDING: 'pending',
  APPROVED: 'approved',
  CREDITED: 'credited',
  REJECTED: 'rejected',
} as const

export const REFUND_STATUS_VALUES: string[] = Object.values(REFUND_STATUS)

export const REFUND_STATUSES: Record<
  string,
  Pick<StatusBadgeProps, 'variant'> & { labelKey: string }
> = {
  [REFUND_STATUS.PENDING]: { labelKey: 'Pending', variant: 'warning' },
  [REFUND_STATUS.APPROVED]: { labelKey: 'Approved', variant: 'info' },
  [REFUND_STATUS.CREDITED]: { labelKey: 'Credited', variant: 'success' },
  [REFUND_STATUS.REJECTED]: { labelKey: 'Rejected', variant: 'neutral' },
}

export function getRefundStatusOptions(t: TFunction) {
  return REFUND_STATUS_VALUES.map((status) => ({
    label: t(REFUND_STATUSES[status].labelKey),
    value: status,
  }))
}

// ============================================================================
// Advance Actions
// ============================================================================

export type RefundAdvanceConfig = {
  action: 'approved' | 'credited' | 'rejected'
  labelKey: string
  destructive?: boolean
}

// Which transitions are offered for a request in the given state. The backend
// refuses anything else, so the menu only advertises the legal moves.
export function getRefundAdvanceActions(status: string): RefundAdvanceConfig[] {
  if (status === REFUND_STATUS.PENDING) {
    return [
      { action: 'approved', labelKey: 'Approve' },
      { action: 'rejected', labelKey: 'Reject', destructive: true },
    ]
  }
  if (status === REFUND_STATUS.APPROVED) {
    return [
      { action: 'credited', labelKey: 'Mark as Credited' },
      { action: 'rejected', labelKey: 'Reject', destructive: true },
    ]
  }
  return []
}

// ============================================================================
// Error Messages
// ============================================================================

// i18n keys; use t(ERROR_MESSAGES.xxx) when displaying.
export const ERROR_MESSAGES = {
  LOAD_FAILED: 'Failed to load refund requests',
  ADVANCE_FAILED: 'Failed to update refund request',
} as const

// i18n keys; use t(SUCCESS_MESSAGES.xxx) when displaying.
export const SUCCESS_MESSAGES = {
  ADVANCED: 'Refund request updated',
} as const
