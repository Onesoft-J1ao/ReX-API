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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/**
 * The three per-group per-token overrides the backend accepts. They are the
 * token-priced counterpart of `group_model_price`, which is per call.
 */
export type GroupModelRatioField = 'ratio' | 'completion' | 'cache'

export type GroupModelRatioData = {
  ratio: Record<string, Record<string, number>>
  completion: Record<string, Record<string, number>>
  cache: Record<string, Record<string, number>>
}

type RatioRow = {
  _id: string
  group: string
  model: string
  ratio: string
  completion: string
  cache: string
}

let _idCounter = 0
function uid() {
  return `gmr_${++_idCounter}`
}

function readNumber(
  map: Record<string, Record<string, number>>,
  group: string,
  model: string
): string {
  const value = map[group]?.[model]
  return typeof value === 'number' && Number.isFinite(value)
    ? String(value)
    : ''
}

/**
 * One row per (group, model) that appears in any of the three maps. A model can
 * be priced in one map without the others, so every group/model pair is
 * unioned across the maps and each cell shows only what that map actually
 * holds.
 */
function flatten(data: GroupModelRatioData): RatioRow[] {
  const seen = new Map<string, Set<string>>()
  for (const map of [data.ratio, data.completion, data.cache]) {
    for (const [group, byModel] of Object.entries(map)) {
      if (!byModel || typeof byModel !== 'object') continue
      const models = seen.get(group) ?? new Set<string>()
      for (const model of Object.keys(byModel)) models.add(model)
      seen.set(group, models)
    }
  }

  const rows: RatioRow[] = []
  for (const [group, models] of seen) {
    for (const model of models) {
      rows.push({
        _id: uid(),
        group,
        model,
        ratio: readNumber(data.ratio, group, model),
        completion: readNumber(data.completion, group, model),
        cache: readNumber(data.cache, group, model),
      })
    }
  }
  return rows
}

/**
 * Build the three maps from the rows. A blank or non-finite cell drops that
 * entry rather than writing NaN: the backend rejects a non-finite ratio and
 * would fail the whole settings save. A blank `ratio` also drops the model's
 * completion/cache, because a multiplier without an input ratio can never
 * apply.
 */
function serialize(rows: RatioRow[]): GroupModelRatioData {
  const ratio: Record<string, Record<string, number>> = {}
  const completion: Record<string, Record<string, number>> = {}
  const cache: Record<string, Record<string, number>> = {}

  const put = (
    target: Record<string, Record<string, number>>,
    group: string,
    model: string,
    raw: string
  ) => {
    const parsed = Number(raw)
    if (!raw.trim() || !Number.isFinite(parsed) || parsed < 0) return
    if (!target[group]) target[group] = {}
    target[group][model] = parsed
  }

  for (const row of rows) {
    if (!row.group || !row.model) continue
    const base = Number(row.ratio)
    if (!row.ratio.trim() || !Number.isFinite(base) || base < 0) continue
    put(ratio, row.group, row.model, row.ratio)
    put(completion, row.group, row.model, row.completion)
    put(cache, row.group, row.model, row.cache)
  }
  return { ratio, completion, cache }
}

function toJson(map: Record<string, Record<string, number>>): string {
  return Object.keys(map).length === 0 ? '{}' : JSON.stringify(map, null, 2)
}

type GroupModelRatioEditorProps = {
  value: GroupModelRatioData
  groupOptions: string[]
  modelOptions: string[]
  onChange: (value: GroupModelRatioData) => void
}

/**
 * Per-group per-token pricing for one model. The three maps override the
 * model's global ratio, output multiplier and cache-read multiplier for the
 * group named here, and the group ratio still multiplies the result.
 */
export function GroupModelRatioEditor(props: GroupModelRatioEditorProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<string[]>([])

  const rows = useMemo(() => flatten(props.value), [props.value])

  const byGroup = useMemo(() => {
    const grouped = new Map<string, RatioRow[]>()
    for (const row of rows) {
      const list = grouped.get(row.group)
      if (list) list.push(row)
      else grouped.set(row.group, [row])
    }
    return grouped
  }, [rows])

  const commit = useCallback(
    (next: RatioRow[]) => {
      const { ratio, completion, cache } = serialize(next)
      props.onChange({
        ratio: JSON.parse(toJson(ratio)),
        completion: JSON.parse(toJson(completion)),
        cache: JSON.parse(toJson(cache)),
      })
    },
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
        ...rows,
        { _id: uid(), group, model: '', ratio: '', completion: '', cache: '' },
      ])
      if (!expanded.includes(group)) setExpanded((c) => [...c, group])
    },
    [commit, expanded, rows]
  )

  const updateRow = useCallback(
    (id: string, patch: Partial<Omit<RatioRow, '_id'>>) => {
      commit(rows.map((row) => (row._id === id ? { ...row, ...patch } : row)))
    },
    [commit, rows]
  )

  const removeRow = useCallback(
    (id: string) => commit(rows.filter((row) => row._id !== id)),
    [commit, rows]
  )

  const removeGroup = useCallback(
    (group: string) => commit(rows.filter((row) => row.group !== group)),
    [commit, rows]
  )

  const groups = useMemo(() => {
    const names = new Set<string>(props.groupOptions)
    for (const group of byGroup.keys()) names.add(group)
    return [...names].sort()
  }, [props.groupOptions, byGroup])

  return (
    <Card className={sectionCardClassName}>
      <CardHeader className={sectionHeaderClassName}>
        <CardTitle className='text-base'>{t('Per-group token pricing')}</CardTitle>
        <CardDescription>
          {t(
            'Override a model’s token ratio, output multiplier and cache-read multiplier inside one group. The group ratio still multiplies the result, so the numbers here are the cost base, not the final charge.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        {groups.map((group) => {
          const groupRows = byGroup.get(group) ?? []
          const open = expanded.includes(group)
          const isKnownGroup = props.groupOptions.includes(group)
          return (
            <Collapsible key={group} open={open} onOpenChange={() => toggleGroup(group)}>
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
                      {groupRows.length} {t('models')}
                    </StatusBadge>
                  </div>
                  <div className='flex items-center gap-1'>
                    <Button
                      variant='ghost'
                      size='sm'
                      disabled={!isKnownGroup}
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
                    {groupRows.length === 0 && (
                      <p className='text-muted-foreground text-xs'>
                        {t('No token overrides in this group.')}
                      </p>
                    )}
                    {groupRows.map((row) => (
                      <RatioRowEditor
                        key={row._id}
                        row={row}
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
        })}
        <AddGroup groupOptions={props.groupOptions} onAdd={addRow} />
      </CardContent>
    </Card>
  )
}

function RatioRowEditor(props: {
  row: RatioRow
  modelOptions: string[]
  onUpdate: (id: string, patch: Partial<Omit<RatioRow, '_id'>>) => void
  onRemove: (id: string) => void
}) {
  const { t } = useTranslation()
  const { row } = props

  return (
    <div className='flex flex-wrap items-end gap-2'>
      <div className='min-w-48 flex-1'>
        <Label className='text-xs'>{t('Model')}</Label>
        <Combobox
          options={props.modelOptions.map((name) => ({
            value: name,
            label: name,
          }))}
          value={row.model}
          onValueChange={(value) => props.onUpdate(row._id, { model: value ?? '' })}
          className='min-w-48 flex-1'
          placeholder={t('Model')}
          aria-label={t('Model')}
        />
      </div>
      <div className='w-28'>
        <Label className='text-xs'>{t('Token ratio')}</Label>
        <Input
          inputMode='decimal'
          value={row.ratio}
          onChange={(event) => props.onUpdate(row._id, { ratio: event.target.value })}
        />
      </div>
      <div className='w-28'>
        <Label className='text-xs'>{t('Output multiplier')}</Label>
        <Input
          inputMode='decimal'
          value={row.completion}
          onChange={(event) =>
            props.onUpdate(row._id, { completion: event.target.value })
          }
        />
      </div>
      <div className='w-28'>
        <Label className='text-xs'>{t('Cache multiplier')}</Label>
        <Input
          inputMode='decimal'
          value={row.cache}
          onChange={(event) => props.onUpdate(row._id, { cache: event.target.value })}
        />
      </div>
      <Button
        variant='ghost'
        size='sm'
        aria-label={t('Remove override')}
        onClick={() => props.onRemove(row._id)}
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
