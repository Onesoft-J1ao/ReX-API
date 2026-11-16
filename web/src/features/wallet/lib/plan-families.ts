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
import type { PlanRecord } from '@/features/subscriptions/types'

export interface PlanTier extends PlanRecord {
  label: string
}

export interface PlanFamily {
  key: string
  tiers: PlanTier[]
  activeIndex: number
}

// Drop the allowance suffix so "Go Plan 5x" groups with "Go Plan".
const TIER_SUFFIX = /\s+(?:5x|20x)$/i

function familyKey(record: PlanRecord): string {
  const plan = record.plan
  const baseTitle = (plan.title || '').replace(TIER_SUFFIX, '').trim()
  // Two plans that unlock different groups are different products even when
  // they share a base title.
  const scope = plan.wallet_only_groups || plan.upgrade_group || ''
  return `${baseTitle}|${scope}`
}

function tierLabel(amount: number, base: number, index: number): string {
  if (base > 0) {
    const ratio = amount / base
    if (ratio >= 1 && Math.abs(ratio - Math.round(ratio)) < 0.01) {
      return `${Math.round(ratio)}x`
    }
  }
  return `${index + 1}x`
}

/**
 * Collapse plans that are the same offer at different allowance levels into a
 * single family, ordered from the smallest allowance up. `activeTier` maps a
 * family key to the tier index the user has selected.
 */
export function groupPlanFamilies(
  plans: PlanRecord[],
  activeTier: Record<string, number> = {}
): PlanFamily[] {
  const families = new Map<string, PlanRecord[]>()
  for (const record of plans) {
    if (!record?.plan) continue
    const key = familyKey(record)
    const bucket = families.get(key)
    if (bucket) bucket.push(record)
    else families.set(key, [record])
  }

  return [...families.entries()].map(([key, records]) => {
    const sorted = [...records].sort(
      (a, b) =>
        Number(a.plan?.total_amount || 0) - Number(b.plan?.total_amount || 0)
    )
    const baseline = Number(sorted[0]?.plan?.total_amount || 0)
    const tiers = sorted.map((record, index) => ({
      ...record,
      label: tierLabel(Number(record.plan?.total_amount || 0), baseline, index),
    }))
    const activeIndex = Math.min(
      activeTier[key] ?? 0,
      Math.max(0, tiers.length - 1)
    )
    return { key, tiers, activeIndex }
  })
}
