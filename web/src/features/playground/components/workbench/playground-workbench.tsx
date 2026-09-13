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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

import { PlaygroundChatMode } from '../chat/playground-chat-mode'
import { ImageWorkbench } from './image-workbench'

// Labels are listed in src/i18n/static-keys.ts because they are selected at
// runtime rather than written as literals next to t().
const WORKBENCH_MODES = [
  { value: 'chat', label: 'Chat', ready: true },
  { value: 'image', label: 'Image', ready: true },
  { value: 'video', label: 'Video', ready: false },
  { value: 'audio', label: 'Speech', ready: false },
  { value: '3d', label: '3D', ready: false },
] as const

type WorkbenchMode = (typeof WORKBENCH_MODES)[number]['value']

export function PlaygroundWorkbench() {
  const { t } = useTranslation()
  const [mode, setMode] = useState<WorkbenchMode>('chat')

  return (
    <div className='flex size-full min-h-0 flex-col overflow-hidden'>
      <div className='shrink-0 border-b px-4 py-2 sm:px-6'>
        <div className='bg-muted mx-auto flex w-fit max-w-full items-center overflow-x-auto rounded-md p-1'>
          {WORKBENCH_MODES.map((item) => (
            <Button
              key={item.value}
              className='shrink-0 gap-1.5'
              onClick={() => setMode(item.value)}
              size='sm'
              variant={mode === item.value ? 'secondary' : 'ghost'}
            >
              {t(item.label)}
              {!item.ready && (
                <Badge
                  className='px-1.5 py-0 text-[0.625rem]'
                  variant='outline'
                >
                  {t('Soon')}
                </Badge>
              )}
            </Button>
          ))}
        </div>
      </div>

      <div className='flex min-h-0 flex-1 flex-col overflow-hidden'>
        {mode === 'chat' ? (
          <PlaygroundChatMode />
        ) : mode === 'image' ? (
          <ImageWorkbench />
        ) : (
          <div className='text-muted-foreground flex min-h-0 flex-1 items-center justify-center p-8 text-center text-sm'>
            {t('This workbench mode is on the way.')}
          </div>
        )}
      </div>
    </div>
  )
}
