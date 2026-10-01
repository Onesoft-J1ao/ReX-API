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
import { Play, Server } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

import { suggestedCount } from '../lib/draft'
import type { RegistrarProvider, RegistrarProviderConfig } from '../types'

interface RegistrarProviderCardProps {
  provider: RegistrarProvider
  draft: RegistrarProviderConfig
  running: boolean
  onDraftChange: (next: RegistrarProviderConfig) => void
  onRun: (count: number) => void
}

/** One registration source: its policy fields, live key counts, and run control. */
export function RegistrarProviderCard(props: RegistrarProviderCardProps) {
  const { t } = useTranslation()
  const [runCount, setRunCount] = useState(() => suggestedCount(props.provider))

  const channel = props.provider.channel
  const belowFloor = channel.usable_keys < props.draft.min_keys

  const updateField = <K extends keyof RegistrarProviderConfig>(
    key: K,
    value: RegistrarProviderConfig[K]
  ) => {
    props.onDraftChange({ ...props.draft, [key]: value })
  }

  return (
    <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
      <div className='flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5'>
        <div className='flex min-w-0 items-center gap-2'>
          <span className='bg-muted text-muted-foreground inline-flex size-7 items-center justify-center rounded-md'>
            <Server className='size-4' aria-hidden='true' />
          </span>
          <div className='min-w-0'>
            <h3 className='truncate text-sm font-semibold'>
              {props.draft.label || props.provider.id}
            </h3>
            <p className='text-muted-foreground mt-0.5 truncate font-mono text-xs'>
              {props.provider.id} · {props.draft.script_path}
            </p>
          </div>
        </div>
        <div className='flex shrink-0 items-center gap-3'>
          <span
            className={cn(
              'text-muted-foreground inline-flex items-center gap-1.5 text-xs',
              belowFloor && 'text-warning'
            )}
          >
            {t('Usable keys')} {channel.usable_keys}/{channel.total_keys}
          </span>
          <div className='flex items-center gap-2'>
            <Label
              htmlFor={`registrar-enabled-${props.provider.id}`}
              className='text-xs'
            >
              {props.draft.enabled ? t('Enabled') : t('Disabled')}
            </Label>
            <Switch
              id={`registrar-enabled-${props.provider.id}`}
              checked={props.draft.enabled}
              onCheckedChange={(checked) => updateField('enabled', checked)}
            />
          </div>
        </div>
      </div>

      <div className='grid gap-4 p-4 sm:grid-cols-2 sm:p-5 lg:grid-cols-4'>
        <div className='space-y-1.5'>
          <Label htmlFor={`registrar-min-${props.provider.id}`}>
            {t('Key floor')}
          </Label>
          <Input
            id={`registrar-min-${props.provider.id}`}
            type='number'
            min={0}
            value={props.draft.min_keys}
            onChange={(event) =>
              updateField('min_keys', Number(event.target.value))
            }
          />
        </div>
        <div className='space-y-1.5'>
          <Label htmlFor={`registrar-target-${props.provider.id}`}>
            {t('Target keys')}
          </Label>
          <Input
            id={`registrar-target-${props.provider.id}`}
            type='number'
            min={0}
            value={props.draft.target_keys}
            onChange={(event) =>
              updateField('target_keys', Number(event.target.value))
            }
          />
        </div>
        <div className='space-y-1.5'>
          <Label htmlFor={`registrar-batch-${props.provider.id}`}>
            {t('Batch cap')}
          </Label>
          <Input
            id={`registrar-batch-${props.provider.id}`}
            type='number'
            min={1}
            value={props.draft.max_batch}
            onChange={(event) =>
              updateField('max_batch', Number(event.target.value))
            }
          />
        </div>
        <div className='space-y-1.5'>
          <Label htmlFor={`registrar-workers-${props.provider.id}`}>
            {t('Workers')}
          </Label>
          <Input
            id={`registrar-workers-${props.provider.id}`}
            type='number'
            min={1}
            max={32}
            value={props.draft.workers}
            onChange={(event) =>
              updateField('workers', Number(event.target.value))
            }
          />
        </div>
      </div>

      <div className='flex flex-col gap-3 border-t px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5'>
        <div className='flex flex-wrap items-center gap-2'>
          <Badge variant='outline'>
            {t('Channel')} #{channel.id || props.draft.channel_id}
          </Badge>
          {channel.name && <Badge variant='outline'>{channel.name}</Badge>}
          <Badge variant='outline'>
            {channel.multi_key ? t('Multi-key') : t('Single key')}
          </Badge>
          <Badge variant='outline'>
            {t('Archived accounts')} {props.provider.account_count}
          </Badge>
          {belowFloor && (
            <Badge variant='destructive'>{t('Below the key floor')}</Badge>
          )}
        </div>
        <div className='flex shrink-0 items-center gap-2'>
          <Input
            type='number'
            min={1}
            max={props.draft.max_batch}
            className='w-24'
            aria-label={t('How many accounts to register')}
            value={runCount}
            onChange={(event) => setRunCount(Number(event.target.value))}
          />
          <Button
            type='button'
            size='sm'
            disabled={props.running}
            onClick={() => props.onRun(runCount)}
          >
            <Play data-icon='inline-start' className='size-3.5' aria-hidden='true' />
            {props.running ? t('Registration running') : t('Register now')}
          </Button>
        </div>
      </div>
    </section>
  )
}
