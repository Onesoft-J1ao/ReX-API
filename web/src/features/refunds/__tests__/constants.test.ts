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

import {
  REFUND_KINDS,
  REFUND_STATUS,
  REFUND_STATUSES,
  REFUND_STATUS_VALUES,
  getRefundAdvanceActions,
} from '../constants'

describe('refund status metadata', () => {
  it('describes every status the backend can return', () => {
    for (const status of REFUND_STATUS_VALUES) {
      expect(REFUND_STATUSES[status]).toBeDefined()
      expect(REFUND_STATUSES[status].labelKey).not.toBe('')
    }
  })

  it('covers the four workflow states and nothing else', () => {
    expect(REFUND_STATUS_VALUES).toEqual([
      REFUND_STATUS.PENDING,
      REFUND_STATUS.APPROVED,
      REFUND_STATUS.CREDITED,
      REFUND_STATUS.REJECTED,
    ])
  })

  it('labels both refund kinds', () => {
    expect(Object.keys(REFUND_KINDS).sort()).toEqual(['cash', 'plan'])
  })
})

describe('getRefundAdvanceActions', () => {
  // The menu must offer exactly the transitions model.AdvanceRefundRequest
  // accepts, or the UI shows an action the backend rejects.
  it('offers approval and rejection while a request is pending', () => {
    expect(getRefundAdvanceActions(REFUND_STATUS.PENDING)).toEqual([
      { action: 'approved', labelKey: 'Approve' },
      { action: 'rejected', labelKey: 'Reject', destructive: true },
    ])
  })

  it('offers only credential confirmation and rejection once approved', () => {
    expect(getRefundAdvanceActions(REFUND_STATUS.APPROVED)).toEqual([
      { action: 'credited', labelKey: 'Mark as Credited' },
      { action: 'rejected', labelKey: 'Reject', destructive: true },
    ])
  })

  it.each([REFUND_STATUS.CREDITED, REFUND_STATUS.REJECTED])(
    'offers nothing for the terminal state %s',
    (status) => {
      expect(getRefundAdvanceActions(status)).toEqual([])
    }
  )

  it('offers nothing for a status it does not recognise', () => {
    expect(getRefundAdvanceActions('unknown')).toEqual([])
    expect(getRefundAdvanceActions('')).toEqual([])
  })
})
