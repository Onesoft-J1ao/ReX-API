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
import { api } from '@/lib/http-client'

/** A distributor group's per-group price multipliers, keyed by local group. */
export type DistributorOverrides = Record<string, Record<string, number>>

export type DistributorData = {
  /** Every distributor group and the multipliers it overrides. */
  overrides: DistributorOverrides
  /** Every group that can be priced; the editor's row list. */
  groups: string[]
  /** Just the keys of `overrides`, in stable order. */
  distributors: string[]
}

type DistributorResponse = {
  success: boolean
  message?: string
  data: DistributorData
}

export async function getDistributors(): Promise<DistributorData> {
  const res = await api.get<DistributorResponse>('/api/group/distributor')
  return res.data.data
}

export async function saveDistributor(
  userGroup: string,
  ratios: Record<string, number>
): Promise<void> {
  await api.put('/api/group/distributor', {
    user_group: userGroup,
    ratios,
  })
}

export async function deleteDistributor(userGroup: string): Promise<void> {
  await api.delete('/api/group/distributor', {
    params: { user_group: userGroup },
  })
}

/** One user who can be promoted to, or demoted from, a distributor group. */
export type DistributorUser = {
  id: number
  username: string
  email: string
  group: string
  quota: number
  status: number
}

export type DistributorMembers = {
  groups: string[]
  members: DistributorUser[]
}

export async function getDistributorMembers(): Promise<DistributorMembers> {
  const res = await api.get<{ success: boolean; data: DistributorMembers }>(
    '/api/group/distributor/members'
  )
  return res.data.data
}

/**
 * Find users to promote. An empty query returns the newest users, an all-digit
 * query matches an id exactly, anything else matches username or email.
 */
export async function searchDistributorCandidates(
  query: string
): Promise<DistributorUser[]> {
  const res = await api.get<{ success: boolean; data: DistributorUser[] }>(
    '/api/group/distributor/candidates',
    { params: { q: query } }
  )
  return res.data.data ?? []
}

/** Move a user into a distributor group; an empty group restores the default. */
export async function assignDistributor(
  userId: number,
  userGroup: string
): Promise<{ previous: string; group: string }> {
  const res = await api.put<{
    success: boolean
    data: { previous: string; group: string }
  }>('/api/group/distributor/member', {
    user_id: userId,
    user_group: userGroup,
  })
  return res.data.data
}
