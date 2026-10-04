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
import { ChevronDown, ChevronUp, Plus, Trash2 } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Combobox } from '@/components/ui/combobox'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

/**
 * The pricing sources the backend accepts. They are not billing modes: the
 * per-call path is selected by whether a price is configured rather than by a
 * mode value, so it needs its own name here.
 */
const PRICING_SOURCES = ['ratio', 'price', 'tiered_expr'] as const
type PricingSource = (typeof PRICING_SOURCES)[number]

function isPricingSource(value: string): value is PricingSource {
  return (PRICING_SOURCES as readonly string[]).includes(value)
}

type Override = {
  _id: string
  group: string
  model: string
  source: PricingSource
}

let _idCounter = 0
function uid() {
  return `gps_${++_idCounter}`
}

function safeParse(raw: string): Record<string, Record<string, string>> {
  if (!raw.trim()) return {}
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return parsed as Record<string, Record<string, string>>
  } catch {
    return {}
  }
}

function flattenOverrides(
  nested: Record<string, Record<string, string>>
): Override[] {
  const overrides: Override[] = []
  for (const [group, byModel] of Object.entries(nested)) {
    if (!byModel || typeof byModel !== 'object') continue
    for (const [model, source] of Object.entries(byModel)) {
      if (typeof source !== 'string' || !isPricingSource(source)) continue
      overrides.push({ _id: uid(), group, model, source })
    }
  }
  return overrides
}

function serializeOverrides(overrides: Override[]): string {
  const result: Record<string, Record<string, string>> = {}
  for (const { group, model, source } of overrides) {
    if (!group || !model) continue
    if (!result[group]) result[group] = {}
    result[group][model] = source
  }
  return Object.keys(result).length === 0
    ? '{}'
    : JSON.stringify(result, null, 2)
}

type GroupPricingSourceEditorProps = {
  value: string
  groupOptions: string[]
  modelOptions: string[]
  onChange: (value: string) => void
}

export function GroupPricingSourceEditor(props: GroupPricingSourceEditorProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<string[]>([])

  const overrides = useMemo(
    () => flattenOverrides(safeParse(props.value)),
    [props.value]
  )

  const byGroup = useMemo(() => {
    const grouped = new Map<string, Override[]>()
    for (const override of overrides) {
      const list = grouped.get(override.group)
      if (list) list.push(override)
      else grouped.set(override.group, [override])
    }
    return grouped
  }, [overrides])

  const commit = useCallback(
    (next: Override[]) => props.onChange(serializeOverrides(next)),
    [props]
  )

  const toggleGroup = useCallback((group: string) => {
    setExpanded((current) =>
      current.includes(group)
        ? current.filter((name) => name !== group)
        : [...current, group]
    )
  }, [])

  const addRow = useCallback(
    (group: string) => {
      commit([
        ...overrides,
        { _id: uid(), group, model: '', source: 'ratio' },
      ])
      if (!expanded.includes(group)) setExpanded((c) => [...c, group])
    },
    [commit, expanded, overrides]
  )

  const updateRow = useCallback(
    (id: string, patch: Partial<Omit<Override, '_id'>>) => {
      commit(
        overrides.map((override) =>
          override._id === id ? { ...override, ...patch } : override
        )
      )
    },
    [commit, overrides]
  )

  const removeRow = useCallback(
    (id: string) => commit(overrides.filter((override) => override._id !== id)),
    [commit, overrides]
  )

  const removeGroup = useCallback(
    (group: string) =>
      commit(overrides.filter((override) => override.group !== group)),
    [commit, overrides]
  )

  return (
    <Card className={sectionCardClassName}>
      <CardHeader className={sectionHeaderClassName}>
        <CardTitle className='text-base'>{t('Per-group billing')}</CardTitle>
        <CardDescription>
          {t(
            'Price one model differently in a given group. A group with no override uses each model’s global billing mode.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        {byGroup.size === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t('No group overrides. Every model bills the same way everywhere.')}
          </p>
        ) : (
          [...byGroup.entries()].map(([group, items]) => {
            const open = expanded.includes(group)
            const isKnownGroup = props.groupOptions.includes(group)
            return (
              <Collapsible
                key={group}
                open={open}
                onOpenChange={() => toggleGroup(group)}
              >
                <div className='rounded-xl border'>
                  <div className='flex items-center justify-between p-3'>
                    <div className='flex items-center gap-2'>
                      <CollapsibleTrigger
                        render={
                          <Button
                            variant='ghost'
                            size='sm'
                            className='h-7 w-7 p-0'
                            aria-label={t('Details')}
                          />
                        }
                      >
                        {open ? (
                          <ChevronUp className='h-4 w-4' />
                        ) : (
                          <ChevronDown className='h-4 w-4' />
                        )}
                      </CollapsibleTrigger>
                      <span className='font-semibold'>{group}</span>
                      {!isKnownGroup && (
                        <StatusBadge variant='danger' copyable={false}>
                          {t('No group ratio — this override never applies')}
                        </StatusBadge>
                      )}
                      <StatusBadge variant='neutral' copyable={false}>
                        {items.length} {t('models')}
                      </StatusBadge>
                    </div>
                    <div className='flex items-center gap-1'>
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => addRow(group)}
                      >
                        <Plus />
                        {t('Add model')}
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        aria-label={t('Remove group')}
                        onClick={() => removeGroup(group)}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </div>
                  <CollapsibleContent>
                    <div className='space-y-2 border-t p-3'>
                      {items.map((override) => (
                        <OverrideRow
                          key={override._id}
                          override={override}
                          modelOptions={props.modelOptions}
                          onUpdate={updateRow}
                          onRemove={removeRow}
                        />
                      ))}
                    </div>
                  </CollapsibleContent>
                </div>
              </Collapsible>
            )
          })
        )}
        <AddGroup
          groupOptions={props.groupOptions}
          onAdd={(group) => addRow(group)}
        />
      </CardContent>
    </Card>
  )
}

function OverrideRow(props: {
  override: Override
  modelOptions: string[]
  onUpdate: (id: string, patch: Partial<Omit<Override, '_id'>>) => void
  onRemove: (id: string) => void
}) {
  const { t } = useTranslation()
  const { override } = props

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <Combobox
        options={props.modelOptions.map((name) => ({
          value: name,
          label: name,
        }))}
        value={override.model}
        onValueChange={(value) =>
          props.onUpdate(override._id, { model: value ?? '' })
        }
        className='min-w-48 flex-1'
        placeholder={t('Model')}
        aria-label={t('Model')}
      />
      <Select
        value={override.source}
        onValueChange={(value) => {
          if (value && isPricingSource(value)) {
            props.onUpdate(override._id, { source: value })
          }
        }}
      >
        <SelectTrigger className='w-44' aria-label={t('Billing source')}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value='ratio'>
              {t('Per token (model ratio)')}
            </SelectItem>
            <SelectItem value='price'>
              {t('Per request (model price)')}
            </SelectItem>
            <SelectItem value='tiered_expr'>
              {t('Expression (tiered)')}
            </SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
      <Button
        variant='ghost'
        size='sm'
        aria-label={t('Remove override')}
        onClick={() => props.onRemove(override._id)}
      >
        <Trash2 />
      </Button>
    </div>
  )
}

function AddGroup(props: {
  groupOptions: string[]
  onAdd: (group: string) => void
}) {
  const { t } = useTranslation()
  const [group, setGroup] = useState('')

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <Combobox
        options={props.groupOptions.map((name) => ({
          value: name,
          label: name,
        }))}
        value={group}
        onValueChange={(value) => setGroup(value ?? '')}
        className='min-w-48'
        placeholder={t('Group')}
        aria-label={t('Group')}
      />
      <Button
        variant='outline'
        size='sm'
        disabled={!group}
        onClick={() => {
          props.onAdd(group)
          setGroup('')
        }}
      >
        <Plus />
        {t('Add override')}
      </Button>
    </div>
  )
}

const sectionCardClassName =
  'relative shadow-sm ring-0 before:pointer-events-none before:absolute before:inset-0 before:rounded-xl before:border before:border-border/90'
const sectionHeaderClassName = 'border-b bg-muted/20'