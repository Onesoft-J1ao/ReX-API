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
import { useParams } from '@tanstack/react-router'
import { ExternalLink } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { TOOLS } from './tools-data'

export function ToolsPage() {
  const { t } = useTranslation()
  const { tool } = useParams({ from: '/_authenticated/tools/$tool' })
  const current = TOOLS.find((x) => x.id === tool) ?? TOOLS[0]

  return (
    <div className='flex h-full flex-col'>
      {/* Header */}
      <div className='flex items-center justify-between border-b px-4 py-3 sm:px-6'>
        <div>
          <h1 className='text-lg font-semibold'>{current.name}</h1>
          <p className='text-muted-foreground text-xs'>{current.description}</p>
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={() =>
            window.open(current.url, '_blank', 'noopener,noreferrer')
          }
        >
          <ExternalLink className='mr-1.5 size-3.5' />
          {t('Open in new tab')}
        </Button>
      </div>

      {/* iframe */}
      <iframe
        key={current.id}
        src={current.url}
        title={current.name}
        className='w-full flex-1 border-0'
        // sandbox is intentionally permissive for local trusted tools
        // eslint-disable-next-line react/iframe-missing-sandbox
      />
    </div>
  )
}
