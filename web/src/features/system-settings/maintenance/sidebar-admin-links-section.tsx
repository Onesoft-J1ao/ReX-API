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
import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  type SidebarAdminLink,
  normalizeSidebarAdminLink,
} from '@/lib/nav-modules'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type DraftLink = {
  id: string
  title: string
  url: string
}

type SidebarAdminLinksSectionProps = {
  links: SidebarAdminLink[]
  initialSerialized: string
}

// Rows are edited in place and have no natural identity, so each draft gets a
// sequence number for its React key. It never leaves the component.
let rowSequence = 0
const nextRowId = () => `admin-link-${++rowSequence}`

const toDraft = (links: SidebarAdminLink[]): DraftLink[] =>
  links.map((link) => ({ ...link, id: nextRowId() }))

/**
 * Editor for the external admin tools shown in the sidebar.
 *
 * These point at services the operator runs outside this console, so the
 * addresses live here rather than in the source. Rows that do not validate
 * (missing title, or not an absolute http/https address) are dropped on save
 * instead of being stored, so the sidebar never has to render a broken entry.
 */
export function SidebarAdminLinksSection({
  links,
  initialSerialized,
}: SidebarAdminLinksSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [draft, setDraft] = useState<DraftLink[]>(() => toDraft(links))

  useEffect(() => {
    setDraft(toDraft(links))
  }, [links])

  const normalized = draft
    .map((row) => normalizeSidebarAdminLink(row))
    .filter((link): link is SidebarAdminLink => link !== null)
  const serialized = JSON.stringify(normalized)
  const isDirty = serialized !== initialSerialized

  const updateRow = (index: number, patch: Partial<DraftLink>) => {
    setDraft((rows) =>
      rows.map((row, i) => (i === index ? { ...row, ...patch } : row))
    )
  }

  const onSubmit = async () => {
    if (!isDirty) return
    await updateOption.mutateAsync({
      key: 'SidebarAdminLinks',
      value: serialized,
    })
  }

  return (
    <SettingsSection title={t('Sidebar admin links')}>
      <SettingsForm
        onSubmit={(event) => {
          event.preventDefault()
          void onSubmit()
        }}
      >
        <SettingsPageFormActions
          onSave={() => void onSubmit()}
          isSaving={updateOption.isPending}
          isSaveDisabled={!isDirty}
          saveLabel='Save admin links'
        />
        <p className='text-muted-foreground text-sm'>
          {t(
            'External tools shown to administrators in the sidebar. Only absolute http(s) addresses are kept.'
          )}
        </p>
        <div className='flex flex-col gap-3'>
          {draft.map((row, index) => (
            <div
              key={row.id}
              className='flex flex-col gap-2 sm:flex-row sm:items-end'
            >
              <div className='flex flex-1 flex-col gap-1.5'>
                <Label htmlFor={`admin-link-title-${index}`}>
                  {t('Title')}
                </Label>
                <Input
                  id={`admin-link-title-${index}`}
                  value={row.title}
                  placeholder={t('Tool name')}
                  onChange={(event) =>
                    updateRow(index, { title: event.target.value })
                  }
                />
              </div>
              <div className='flex flex-[2] flex-col gap-1.5'>
                <Label htmlFor={`admin-link-url-${index}`}>{t('URL')}</Label>
                <Input
                  id={`admin-link-url-${index}`}
                  value={row.url}
                  placeholder='https://example.com/admin'
                  onChange={(event) =>
                    updateRow(index, { url: event.target.value })
                  }
                />
              </div>
              <Button
                type='button'
                size='icon'
                variant='ghost'
                aria-label={t('Remove link')}
                onClick={() =>
                  setDraft((rows) => rows.filter((_, i) => i !== index))
                }
              >
                <Trash2 />
              </Button>
            </div>
          ))}
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='w-fit'
          onClick={() =>
            setDraft((rows) => [
              ...rows,
              { id: nextRowId(), title: '', url: '' },
            ])
          }
        >
          <Plus />
          {t('Add link')}
        </Button>
      </SettingsForm>
    </SettingsSection>
  )
}
