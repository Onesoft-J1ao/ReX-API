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
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'

import { PLAYER_SPEEDS } from '../lib/player-preference'

export function BangumiPlayerSettings(props: {
  speed: number
  onSpeedChange: (speed: number) => void
  danmakuEnabled: boolean
  onDanmakuEnabledChange: (enabled: boolean) => void
  blockedWords: string
  onBlockedWordsChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const danmakuId = useId()
  const speedId = useId()
  const blockedId = useId()

  return (
    <div className='flex flex-wrap items-end justify-between gap-3'>
      <div className='flex items-center gap-2'>
        <Switch
          id={danmakuId}
          size='sm'
          checked={props.danmakuEnabled}
          onCheckedChange={props.onDanmakuEnabledChange}
        />
        <Label htmlFor={danmakuId} className='text-sm'>
          {t('Danmaku')}
        </Label>
      </div>
      <div className='flex items-center gap-2'>
        <Label htmlFor={speedId} className='text-sm'>
          {t('Playback speed')}
        </Label>
        <NativeSelect
          id={speedId}
          size='sm'
          value={String(props.speed)}
          onChange={(event) => props.onSpeedChange(Number(event.target.value))}
        >
          {PLAYER_SPEEDS.map((speed) => (
            <NativeSelectOption key={speed} value={String(speed)}>
              {`${speed}×`}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </div>
      {props.danmakuEnabled && (
        <div className='flex w-full items-center gap-2 sm:w-auto sm:flex-1'>
          <Label htmlFor={blockedId} className='shrink-0 text-sm'>
            {t('Blocked words')}
          </Label>
          <Input
            id={blockedId}
            value={props.blockedWords}
            onChange={(event) => props.onBlockedWordsChange(event.target.value)}
            placeholder={t('Separate words with a comma')}
            className='h-8'
          />
        </div>
      )}
    </div>
  )
}
