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
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  deleteDistributor,
  getDistributors,
  type DistributorData,
} from './api'
import { DistributorEditor } from './components/distributor-editor'

const QUERY_KEY = ['distributors']

export function Distributors() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<string | null>(null)

  const query = useQuery({
    queryKey: QUERY_KEY,
    queryFn: async () => requireServerSuccess(await getDistributors()),
    staleTime: 30 * 1000,
  })

  const remove = useMutation({
    mutationFn: (userGroup: string) => deleteDistributor(userGroup),
    onSuccess: async (_data, userGroup) => {
      toast.success(t('Removed distributor {{name}}', { name: userGroup }))
      setDeleting(null)
      await queryClient.invalidateQueries({ queryKey: QUERY_KEY })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const data: DistributorData | undefined = query.data
  const distributors = data?.distributors ?? []
  // Stable identities so the editor's effects do not see a new object on every
  // parent render and reset what the operator is typing.
  const editorGroups = useMemo(() => data?.groups ?? [], [data?.groups])
  const editorExisting = useMemo(
    () => data?.overrides ?? {},
    [data?.overrides]
  )

  const editorOpen = creating || editing !== null

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Distributors')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button size='sm' onClick={() => setCreating(true)}>
          <Plus className='mr-2 h-4 w-4' />
          {t('New distributor')}
        </Button>
      </SectionPageLayout.Actions>

      <SectionPageLayout.Content>
        <p className='text-muted-foreground mb-4 text-sm'>
          {t(
            'A distributor group pays a different multiplier per group. Members of that group are charged the override instead of the group’s normal ratio.'
          )}
        </p>
        {query.isLoading ? (
          <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-3'>
            {Array.from({ length: 3 }).map((_, index) => (
              <Skeleton key={index} className='h-40 w-full' />
            ))}
          </div>
        ) : distributors.length === 0 ? (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>{t('No distributors yet')}</EmptyTitle>
              <EmptyDescription>
                {t(
                  'Create one to give a user group its own price multipliers.'
                )}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-3'>
            {distributors.map((userGroup) => {
              const ratios = data?.overrides[userGroup] ?? {}
              const entries = Object.entries(ratios).sort(([a], [b]) =>
                a.localeCompare(b)
              )
              return (
                <Card key={userGroup}>
                  <CardHeader className='flex flex-row items-start justify-between gap-2'>
                    <CardTitle className='text-base break-words'>
                      {userGroup}
                    </CardTitle>
                    <div className='flex shrink-0 gap-1'>
                      <Button
                        variant='ghost'
                        size='icon'
                        aria-label={t('Edit')}
                        onClick={() => setEditing(userGroup)}
                      >
                        <Pencil className='h-4 w-4' />
                      </Button>
                      <Button
                        variant='ghost'
                        size='icon'
                        aria-label={t('Delete')}
                        onClick={() => setDeleting(userGroup)}
                      >
                        <Trash2 className='h-4 w-4' />
                      </Button>
                    </div>
                  </CardHeader>
                  <CardContent className='flex flex-col gap-2'>
                    <p className='text-muted-foreground text-xs'>
                      {t('{{count}} groups overridden', {
                        count: entries.length,
                      })}
                    </p>
                    <div className='flex flex-wrap gap-1.5'>
                      {entries.map(([group, ratio]) => (
                        <Badge key={group} variant='secondary'>
                          {group} ×{ratio}
                        </Badge>
                      ))}
                    </div>
                  </CardContent>
                </Card>
              )
            })}
          </div>
        )}
      </SectionPageLayout.Content>

      <DistributorEditor
        open={editorOpen}
        userGroup={editing}
        groups={editorGroups}
        existing={editorExisting}
        onOpenChange={(open) => {
          if (!open) {
            setCreating(false)
            setEditing(null)
          }
        }}
        onSaved={async () => {
          setCreating(false)
          setEditing(null)
          await queryClient.invalidateQueries({ queryKey: QUERY_KEY })
        }}
      />

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null)
        }}
        title={t('Delete distributor')}
        desc={t(
          'Members of {{name}} will fall back to each group’s normal ratio.',
          { name: deleting ?? '' }
        )}
        destructive
        isLoading={remove.isPending}
        handleConfirm={() => {
          if (deleting) remove.mutate(deleting)
        }}
      />
    </SectionPageLayout>
  )
}
