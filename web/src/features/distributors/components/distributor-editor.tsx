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
import { useMutation } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'

import { saveDistributor, type DistributorOverrides } from '../api'

export interface DistributorEditorProps {
  open: boolean
  /** Existing group being edited; null when creating a new one. */
  userGroup: string | null
  groups: string[]
  existing: DistributorOverrides
  onOpenChange: (open: boolean) => void
  onSaved: () => void | Promise<void>
}

/** Draft state is text so a half-typed "0." is not coerced to a number. */
type Draft = Record<string, string>

export function DistributorEditor(props: DistributorEditorProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [draft, setDraft] = useState<Draft>({})
  const [fill, setFill] = useState('')

  // `existing` is rebuilt on every parent render (a fresh `?? {}`), so keying
  // this effect on the object would reset the draft while the operator is
  // typing. Key it on the identity of what is being edited instead, and read
  // `existing` through a ref so the reset only happens when the editor opens
  // or switches to a different group.
  const existingRef = useRef(props.existing)
  existingRef.current = props.existing

  useEffect(() => {
    if (!props.open) return
    if (props.userGroup) {
      setName(props.userGroup)
      const rows = existingRef.current[props.userGroup] ?? {}
      setDraft(
        Object.fromEntries(
          Object.entries(rows).map(([group, ratio]) => [group, String(ratio)])
        )
      )
    } else {
      setName('')
      setDraft({})
    }
    setFill('')
  }, [props.open, props.userGroup])

  const filled = useMemo(
    () => Object.values(draft).filter((value) => value.trim() !== '').length,
    [draft]
  )

  const save = useMutation({
    mutationFn: async () => {
      const ratios: Record<string, number> = {}
      for (const [group, raw] of Object.entries(draft)) {
        const value = raw.trim()
        if (value === '') continue
        const parsed = Number(value)
        if (!Number.isFinite(parsed) || parsed <= 0) {
          throw new Error(
            t('{{group}} needs a positive number', { group })
          )
        }
        ratios[group] = parsed
      }
      if (Object.keys(ratios).length === 0) {
        throw new Error(t('Fill in at least one group'))
      }
      await saveDistributor(name.trim(), ratios)
    },
    onSuccess: async () => {
      toast.success(t('Saved distributor {{name}}', { name: name.trim() }))
      await props.onSaved()
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const applyFill = () => {
    const value = fill.trim()
    if (value === '') return
    setDraft((prev) => {
      const next: Draft = { ...prev }
      for (const group of props.groups) next[group] = value
      return next
    })
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className='flex w-full flex-col sm:max-w-lg'>
        <SheetHeader>
          <SheetTitle>
            {props.userGroup
              ? t('Edit distributor {{name}}', { name: props.userGroup })
              : t('New distributor')}
          </SheetTitle>
          <SheetDescription>
            {t(
              'The group name is the user group its members belong to. Each multiplier replaces that group’s normal ratio for the listed group.'
            )}
          </SheetDescription>
        </SheetHeader>

        <div className='flex min-h-0 flex-1 flex-col gap-4 px-1'>
          <div className='flex flex-col gap-2'>
            <Label htmlFor='distributor-name'>{t('User group')}</Label>
            <Input
              id='distributor-name'
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder='distributor'
              disabled={Boolean(props.userGroup)}
            />
          </div>

          <div className='flex items-end gap-2'>
            <div className='flex flex-1 flex-col gap-2'>
              <Label htmlFor='distributor-fill'>{t('Fill all groups')}</Label>
              <Input
                id='distributor-fill'
                value={fill}
                onChange={(event) => setFill(event.target.value)}
                placeholder='0.08'
                inputMode='decimal'
              />
            </div>
            <Button variant='outline' onClick={applyFill}>
              {t('Apply to all')}
            </Button>
          </div>

          <p className='text-muted-foreground text-xs'>
            {t('{{filled}} of {{total}} groups filled', {
              filled,
              total: props.groups.length,
            })}
          </p>

          <ScrollArea className='min-h-0 flex-1 rounded-md border'>
            <div className='flex flex-col divide-y'>
              {props.groups.map((group) => (
                <div
                  key={group}
                  className='flex items-center gap-3 px-3 py-2'
                >
                  <span className='min-w-0 flex-1 truncate text-sm' title={group}>
                    {group}
                  </span>
                  <Input
                    aria-label={t('Multiplier for {{group}}', { group })}
                    className='h-8 w-24'
                    value={draft[group] ?? ''}
                    onChange={(event) =>
                      setDraft((prev) => ({
                        ...prev,
                        [group]: event.target.value,
                      }))
                    }
                    placeholder='—'
                    inputMode='decimal'
                  />
                </div>
              ))}
            </div>
          </ScrollArea>
        </div>

        <SheetFooter className='flex-row justify-end gap-2'>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={() => save.mutate()}
            disabled={save.isPending || name.trim() === ''}
          >
            {save.isPending ? t('Saving...') : t('Save')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
