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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Search, UserPlus, X } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  assignDistributor,
  getDistributorMembers,
  searchDistributorCandidates,
  type DistributorUser,
} from '../api'

const MEMBERS_KEY = ['distributor-members']
const CANDIDATES_KEY = ['distributor-candidates']

/**
 * Grant or revoke the distributor identity for one user.
 *
 * The multiplier editor next to this answers "how much does a distributor
 * pay"; this answers "who is one". Both were needed because the identity used
 * to be settable only by editing a user's group field by hand, which meant
 * looking the user up elsewhere first.
 *
 * The user picker searches by id, username, or email, and falls back to the
 * newest users when the box is empty — so an operator who already knows the id
 * can paste it and press search, and an operator who does not can browse.
 */
export function DistributorMembers() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [query, setQuery] = useState('')
  const [submitted, setSubmitted] = useState('')
  const [selected, setSelected] = useState<DistributorUser | null>(null)
  const [targetGroup, setTargetGroup] = useState('')

  const membersQuery = useQuery({
    queryKey: MEMBERS_KEY,
    queryFn: async () => requireServerSuccess(await getDistributorMembers()),
    staleTime: 30 * 1000,
  })

  const candidatesQuery = useQuery({
    queryKey: [...CANDIDATES_KEY, submitted],
    queryFn: async () => searchDistributorCandidates(submitted),
    staleTime: 15 * 1000,
  })

  const groups = useMemo(
    () => membersQuery.data?.groups ?? [],
    [membersQuery.data?.groups]
  )
  const members = useMemo(
    () => membersQuery.data?.members ?? [],
    [membersQuery.data?.members]
  )
  const candidates = useMemo(
    () => candidatesQuery.data ?? [],
    [candidatesQuery.data]
  )

  // Default the target to the first distributor group once it is known, so the
  // common case is pick-user → confirm.
  useEffect(() => {
    if (!targetGroup && groups.length > 0) setTargetGroup(groups[0])
  }, [groups, targetGroup])

  const assign = useMutation({
    mutationFn: (payload: { userId: number; group: string }) =>
      assignDistributor(payload.userId, payload.group),
    onSuccess: async (data) => {
      toast.success(
        data.group === 'default'
          ? t('Revoked distributor identity')
          : t('Assigned to {{group}}', { group: data.group })
      )
      setSelected(null)
      await queryClient.invalidateQueries({ queryKey: MEMBERS_KEY })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const runSearch = () => setSubmitted(query.trim())

  return (
    <Card>
      <CardHeader>
        <CardTitle className='text-base'>{t('Distributor members')}</CardTitle>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Search a user by id, username or email, then assign a distributor group. An empty search lists the newest users.'
          )}
        </p>

        <div className='flex flex-col gap-2 sm:flex-row sm:items-end'>
          <div className='flex flex-1 flex-col gap-2'>
            <Label htmlFor='distributor-search'>{t('User')}</Label>
            <div className='flex gap-2'>
              <Input
                id='distributor-search'
                value={query}
                placeholder={t('User id, username or email')}
                onChange={(event) => setQuery(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    runSearch()
                  }
                }}
              />
              <Button variant='outline' onClick={runSearch}>
                <Search className='mr-1 h-4 w-4' />
                {t('Search')}
              </Button>
            </div>
          </div>
          <div className='flex w-full flex-col gap-2 sm:w-56'>
            <Label>{t('Assign to group')}</Label>
            <Select
              value={targetGroup}
              onValueChange={(value) => value && setTargetGroup(value)}
            >
              <SelectTrigger>
                <SelectValue placeholder={t('Select a distributor group')} />
              </SelectTrigger>
              <SelectContent>
                {groups.map((group) => (
                  <SelectItem key={group} value={group}>
                    {group}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        {candidates.length > 0 && (
          <div className='flex flex-col divide-y rounded-md border'>
            {candidates.map((user) => {
              const isMember = members.some((m) => m.id === user.id)
              const active = selected?.id === user.id
              return (
                <button
                  key={user.id}
                  type='button'
                  onClick={() => setSelected(active ? null : user)}
                  className={`flex items-center gap-3 px-3 py-2 text-left text-sm transition-colors ${
                    active ? 'bg-accent' : 'hover:bg-accent/50'
                  }`}
                >
                  <span className='w-16 shrink-0 font-mono text-xs text-muted-foreground'>
                    #{user.id}
                  </span>
                  <span className='min-w-0 flex-1'>
                    <span className='block truncate font-medium'>
                      {user.username || '—'}
                    </span>
                    <span className='block truncate text-xs text-muted-foreground'>
                      {user.email || t('No email')}
                    </span>
                  </span>
                  <Badge variant={isMember ? 'default' : 'secondary'}>
                    {user.group || 'default'}
                  </Badge>
                </button>
              )
            })}
          </div>
        )}

        {selected && (
          <div className='flex flex-wrap items-center gap-2 rounded-md border bg-muted/40 px-3 py-2'>
            <span className='text-sm'>
              {t('Selected')}: <strong>#{selected.id}</strong>{' '}
              {selected.username}
            </span>
            <div className='ml-auto flex gap-2'>
              <Button
                size='sm'
                disabled={!targetGroup || assign.isPending}
                onClick={() =>
                  assign.mutate({ userId: selected.id, group: targetGroup })
                }
              >
                <UserPlus className='mr-1 h-4 w-4' />
                {t('Assign {{group}}', { group: targetGroup })}
              </Button>
              <Button
                size='sm'
                variant='outline'
                disabled={assign.isPending}
                onClick={() =>
                  assign.mutate({ userId: selected.id, group: '' })
                }
              >
                <X className='mr-1 h-4 w-4' />
                {t('Revoke')}
              </Button>
            </div>
          </div>
        )}

        <div className='flex flex-col gap-2'>
          <Label>{t('Current members')}</Label>
          {members.length === 0 ? (
            <p className='text-muted-foreground text-xs'>
              {t('No users in any distributor group yet.')}
            </p>
          ) : (
            <div className='flex flex-col divide-y rounded-md border'>
              {members.map((user) => (
                <div
                  key={user.id}
                  className='flex items-center gap-3 px-3 py-2 text-sm'
                >
                  <span className='w-16 shrink-0 font-mono text-xs text-muted-foreground'>
                    #{user.id}
                  </span>
                  <span className='min-w-0 flex-1 truncate'>
                    {user.username || user.email || '—'}
                  </span>
                  <Badge variant='secondary'>{user.group}</Badge>
                </div>
              ))}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
