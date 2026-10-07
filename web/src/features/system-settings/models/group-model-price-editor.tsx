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
import { ChevronDown, Plus, Trash2 } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

type PriceRow = {
  _id: string
  group: string
  model: string
  price: string
}

let _idCounter = 0
function uid() {
  return `gmp_${++_idCounter}`
}

function safeParse(raw: string): Record<string, Record<string, number>> {
  if (!raw.trim()) return {}
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return parsed as Record<string, Record<string, number>>
  } catch {
    return {}
  }
}

function flatten(nested: Record<string, Record<string, number>>): PriceRow[] {
  const rows: PriceRow[] = []
  for (const [group, byModel] of Object.entries(nested)) {
    if (!byModel || typeof byModel !== 'object') continue
    for (const [model, price] of Object.entries(byModel)) {
      if (typeof price !== 'number' || !Number.isFinite(price)) continue
      rows.push({ _id: uid(), group, model, price: String(price) })
    }
  }
  return rows
}

// A blank or unparseable price is dropped rather than written as NaN: the
// backend rejects a non-finite value, and silently sending one would make the
// whole settings save fail.
function serialize(rows: PriceRow[]): string {
  const result: Record<string, Record<string, number>> = {}
  for (const { group, model, price } of rows) {
    if (!group || !model) continue
    const parsed = Number(price)
    if (!Number.isFinite(parsed) || parsed < 0) continue
    if (!result[group]) result[group] = {}
    result[group][model] = parsed
  }
  return Object.keys(result).length === 0
    ? '{}'
    : JSON.stringify(result, null, 2)
}

type GroupModelPriceEditorProps = {
  value: string
  groupOptions: string[]
  modelOptions: string[]
  onChange: (value: string) => void
}

/**
 * Per-group per-call prices. A model named here is billed per call inside that
 * group at this price, and the group ratio still multiplies the result — the
 * same way it multiplies the model's global per-call price.
 */
export function GroupModelPriceEditor(props: GroupModelPriceEditorProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<string[]>([])

  const rows = useMemo(() => flatten(safeParse(props.value)), [props.value])

  const byGroup = useMemo(() => {
    const grouped = new Map<string, PriceRow[]>()
    for (const row of rows) {
      const list = grouped.get(row.group)
      if (list) list.push(row)
      else grouped.set(row.group, [row])
    }
    return grouped
  }, [rows])

  const commit = useCallback(
    (next: PriceRow[]) => props.onChange(serialize(next)),
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
      commit([...rows, { _id: uid(), group, model: '', price: '' }])
      if (!expanded.includes(group)) setExpanded((c) => [...c, group])
    },
    [commit, expanded, rows]
  )

  const updateRow = useCallback(
    (id: string, patch: Partial<Omit<PriceRow, '_id'>>) => {
      commit(rows.map((row) => (row._id === id ? { ...row, ...patch } : row)))
    },
    [commit, rows]
  )

  const removeRow = useCallback(
    (id: string) => commit(rows.filter((row) => row._id !== id)),
    [commit, rows]
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle className='text-base'>{t('Group model pricing')}</CardTitle>
        <CardDescription>
          {t(
            'Charge one model a different per-call price inside one group. The group ratio still multiplies the result.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        {props.groupOptions.map((group) => {
          const groupRows = byGroup.get(group) ?? []
          const isOpen = expanded.includes(group)
          return (
            <Collapsible
              key={group}
              open={isOpen}
              onOpenChange={() => toggleGroup(group)}
            >
              <div className='flex items-center gap-2'>
                <CollapsibleTrigger className='flex flex-1 items-center gap-2 text-left text-sm font-medium'>
                  <ChevronDown
                    className={`size-4 shrink-0 transition-transform ${isOpen ? '' : '-rotate-90'}`}
                  />
                  <span className='truncate'>{group}</span>
                  <span className='text-muted-foreground text-xs'>
                    {groupRows.length}
                  </span>
                </CollapsibleTrigger>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() => addRow(group)}
                >
                  <Plus className='size-3.5' />
                  {t('Add')}
                </Button>
              </div>
              <CollapsibleContent className='mt-2 space-y-2 pl-6'>
                {groupRows.length === 0 && (
                  <p className='text-muted-foreground text-xs'>
                    {t('No price overrides in this group.')}
                  </p>
                )}
                {groupRows.map((row) => (
                  <div key={row._id} className='flex items-end gap-2'>
                    <div className='min-w-0 flex-1'>
                      <Label className='text-xs'>{t('Model')}</Label>
                      <Combobox
                        value={row.model}
                        options={props.modelOptions.map((model) => ({
                          value: model,
                          label: model,
                        }))}
                        onValueChange={(model) =>
                          updateRow(row._id, { model: model ?? '' })
                        }
                        className='min-w-48 flex-1'
                        placeholder={t('Model')}
                      />
                    </div>
                    <div className='w-32'>
                      <Label className='text-xs'>{t('Price per call')}</Label>
                      <Input
                        inputMode='decimal'
                        value={row.price}
                        onChange={(event) =>
                          updateRow(row._id, { price: event.target.value })
                        }
                      />
                    </div>
                    <Button
                      type='button'
                      size='icon'
                      variant='ghost'
                      aria-label={t('Remove')}
                      onClick={() => removeRow(row._id)}
                    >
                      <Trash2 className='size-4' />
                    </Button>
                  </div>
                ))}
              </CollapsibleContent>
            </Collapsible>
          )
        })}
      </CardContent>
    </Card>
  )
}
