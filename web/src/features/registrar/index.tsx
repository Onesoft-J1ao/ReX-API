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
import { Save } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { listSystemTasks } from '@/features/system-settings/api'

import {
  getRegistrarProviders,
  runRegistrarProvider,
  updateRegistrarProviders,
} from './api'
import { RegistrarAccountsPanel } from './components/accounts-panel'
import { RegistrarProviderCard } from './components/provider-card'
import { toDraft, type RegistrarDraft } from './lib/draft'

const RUN_POLL_INTERVAL_MS = 5000

export function RegistrarSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<RegistrarDraft | null>(null)

  const providersQuery = useQuery({
    queryKey: ['registrar', 'providers'],
    queryFn: getRegistrarProviders,
    retry: false,
  })

  useEffect(() => {
    if (providersQuery.data) {
      setDraft(toDraft(providersQuery.data))
    }
  }, [providersQuery.data])

  // A registration run is a system task, so the page watches the same active
  // list the maintenance panel reads instead of inventing its own status.
  const activeRunsQuery = useQuery({
    queryKey: ['registrar', 'active-runs'],
    queryFn: async () => {
      const res = await listSystemTasks(20, { scope: 'active' })
      return Array.isArray(res.data) ? res.data : []
    },
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.some((task) =>
        String(task.type).startsWith('registrar_')
      )
        ? RUN_POLL_INTERVAL_MS
        : false,
  })

  const activeRunProviderIds = new Set(
    (activeRunsQuery.data ?? [])
      .filter((task) => task.type === 'registrar_run')
      .map((task) => task.payload?.provider_id)
      .filter((id): id is string => typeof id === 'string')
  )

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!draft) return
      await updateRegistrarProviders({
        enabled: draft.enabled,
        interval_minutes: draft.intervalMinutes,
        providers: draft.providers,
      })
    },
    onSuccess: () => {
      toast.success(t('Registrar settings saved.'))
      void queryClient.invalidateQueries({ queryKey: ['registrar'] })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Save failed.'))
    },
  })

  const runMutation = useMutation({
    mutationFn: async (params: { providerId: string; count: number }) =>
      runRegistrarProvider(params.providerId, params.count),
    onSuccess: (result) => {
      toast.success(
        result.created
          ? t('Registration started.')
          : t('A registration run is already in progress.')
      )
      void queryClient.invalidateQueries({ queryKey: ['registrar'] })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Run failed.'))
    },
  })

  if (providersQuery.isLoading) {
    return (
      <div className='space-y-3'>
        <Skeleton className='h-28 w-full rounded-xl' />
        <Skeleton className='h-28 w-full rounded-xl' />
      </div>
    )
  }

  if (providersQuery.isError || !draft || !providersQuery.data) {
    return (
      <ErrorState
        title={t('We could not load the registrar.')}
        description={
          providersQuery.error instanceof Error
            ? providersQuery.error.message
            : undefined
        }
        onRetry={() => {
          void providersQuery.refetch()
        }}
        className='min-h-[260px]'
      />
    )
  }

  const providers = providersQuery.data.providers

  return (
    <div className='space-y-4'>
      <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
        <div className='flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5'>
          <div className='flex min-w-0 items-center gap-3'>
            <div className='flex items-center gap-2'>
              <Switch
                id='registrar-enabled'
                checked={draft.enabled}
                onCheckedChange={(checked) =>
                  setDraft({ ...draft, enabled: checked })
                }
              />
              <Label htmlFor='registrar-enabled' className='text-xs'>
                {t('Auto replenish')}
              </Label>
            </div>
            <div className='flex items-center gap-2'>
              <Label htmlFor='registrar-interval' className='text-xs'>
                {t('Check interval (minutes)')}
              </Label>
              <Input
                id='registrar-interval'
                type='number'
                min={1}
                max={1440}
                className='w-24'
                value={draft.intervalMinutes}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    intervalMinutes: Number(event.target.value),
                  })
                }
              />
            </div>
          </div>
          <div className='flex shrink-0 items-center gap-3'>
            {activeRunProviderIds.size > 0 && (
              <Badge variant='outline'>{t('Registration running')}</Badge>
            )}
            <Button
              type='button'
              size='sm'
              disabled={saveMutation.isPending}
              onClick={() => saveMutation.mutate()}
            >
              <Save data-icon='inline-start' className='size-3.5' aria-hidden='true' />
              {saveMutation.isPending ? t('Saving...') : t('Save')}
            </Button>
          </div>
        </div>
        <p className='text-muted-foreground px-4 py-3 text-xs sm:px-5'>
          {t(
            'Each provider refills its channel when usable keys fall below the floor, up to the batch cap per run.'
          )}
        </p>
      </section>

      {providers.length === 0 ? (
        <div className='text-muted-foreground rounded-xl border border-dashed px-4 py-10 text-center text-sm'>
          {t('No registration providers configured.')}
        </div>
      ) : (
        providers.map((provider, index) => (
          <div key={provider.id} className='space-y-4'>
            <RegistrarProviderCard
              provider={provider}
              draft={draft.providers[index] ?? provider}
              running={activeRunProviderIds.has(provider.id)}
              onDraftChange={(next) =>
                setDraft({
                  ...draft,
                  providers: draft.providers.map((item, itemIndex) =>
                    itemIndex === index ? next : item
                  ),
                })
              }
              onRun={(count) =>
                runMutation.mutate({ providerId: provider.id, count })
              }
            />
            <RegistrarAccountsPanel providerId={provider.id} />
          </div>
        ))
      )}
    </div>
  )
}
