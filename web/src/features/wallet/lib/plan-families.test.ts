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
import { describe, expect, test } from 'vitest'

import type {
  PlanRecord,
  SubscriptionPlan,
} from '@/features/subscriptions/types'

import { groupPlanFamilies } from './plan-families'

function record(
  title: string,
  price: number,
  totalAmount: number,
  walletOnlyGroups = 'Go Plan'
): PlanRecord {
  return {
    plan: {
      id: price,
      title,
      price_amount: price,
      total_amount: totalAmount,
      wallet_only_groups: walletOnlyGroups,
    } as unknown as SubscriptionPlan,
  }
}

const GO_PLAN = [
  record('Go Plan', 30, 30_000_000),
  record('Go Plan 5x', 150, 150_000_000),
  record('Go Plan 20x', 600, 600_000_000),
]

describe('groupPlanFamilies', () => {
  test('collapses one offer sold at three allowances into a single family', () => {
    const families = groupPlanFamilies(GO_PLAN)

    expect(families).toHaveLength(1)
    expect(families[0].tiers.map((tier) => tier.label)).toEqual([
      '1x',
      '5x',
      '20x',
    ])
    expect(families[0].activeIndex).toBe(0)
    expect(families[0].tiers[0].plan.price_amount).toBe(30)
  })

  test('orders tiers by allowance regardless of the API order', () => {
    const shuffled = [GO_PLAN[2], GO_PLAN[0], GO_PLAN[1]]

    const [family] = groupPlanFamilies(shuffled)

    expect(family.tiers.map((tier) => tier.plan.price_amount)).toEqual([
      30, 150, 600,
    ])
  })

  test('applies the selected tier and clamps it to the available range', () => {
    const key = groupPlanFamilies(GO_PLAN)[0].key

    expect(groupPlanFamilies(GO_PLAN, { [key]: 2 })[0].activeIndex).toBe(2)
    expect(groupPlanFamilies(GO_PLAN, { [key]: 9 })[0].activeIndex).toBe(2)
  })

  test('keeps plans that unlock different groups apart', () => {
    const families = groupPlanFamilies([
      record('Go Plan', 30, 30_000_000, 'Go Plan'),
      record('Go Plan', 30, 30_000_000, 'ClaudeMax'),
      record('Team Plan', 60, 60_000_000, 'Go Plan'),
    ])

    expect(families).toHaveLength(3)
  })
})
