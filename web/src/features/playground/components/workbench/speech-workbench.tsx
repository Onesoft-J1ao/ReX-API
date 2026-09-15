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
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelGroupSelector } from '@/components/model-group-selector'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'

import { getUserGroups, getUserModels } from '../../api'
import {
  SPEECH_FORMAT_OPTIONS,
  SPEECH_SPEED_MAX,
  SPEECH_SPEED_MIN,
  SPEECH_VOICE_SUGGESTIONS,
} from '../../constants'
import { useSpeechGeneration } from '../../hooks/use-speech-generation'

export function SpeechWorkbench() {
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const {
    config,
    updateConfig,
    mode,
    setMode,
    text,
    setText,
    file,
    setFile,
    audioUrl,
    transcript,
    isWorking,
    error,
    canRun,
    run,
  } = useSpeechGeneration()

  const { data: groups = [] } = useQuery({
    queryKey: ['playground', 'workbench', 'groups'],
    queryFn: getUserGroups,
  })
  const { data: models = [] } = useQuery({
    queryKey: ['playground', 'workbench', 'models', config.group],
    queryFn: () => getUserModels(config.group),
  })

  useEffect(() => {
    if (
      groups.length > 0 &&
      !groups.some((group) => group.value === config.group)
    ) {
      updateConfig('group', groups[0].value)
    }
  }, [config.group, groups, updateConfig])

  useEffect(() => {
    if (config.model === '' && models.length > 0) {
      updateConfig('model', models[0].value)
    }
  }, [config.model, models, updateConfig])

  const handleAudioFile = (event: ChangeEvent<HTMLInputElement>) => {
    setFile(event.target.files?.[0] ?? null)
    event.target.value = ''
  }

  return (
    <div className='flex size-full min-h-0 flex-col overflow-hidden'>
      <ScrollArea className='min-h-0 flex-1'>
        <div className='mx-auto grid w-full max-w-6xl gap-4 p-4 sm:p-6 lg:grid-cols-[22rem_minmax(0,1fr)]'>
          <Card className='h-fit'>
            <CardHeader>
              <CardTitle>{t('Speech')}</CardTitle>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='bg-muted flex rounded-md p-1'>
                <Button
                  className='flex-1'
                  onClick={() => setMode('synthesize')}
                  size='sm'
                  variant={mode === 'synthesize' ? 'secondary' : 'ghost'}
                >
                  {t('Text to speech')}
                </Button>
                <Button
                  className='flex-1'
                  onClick={() => setMode('transcribe')}
                  size='sm'
                  variant={mode === 'transcribe' ? 'secondary' : 'ghost'}
                >
                  {t('Speech to text')}
                </Button>
              </div>

              <ModelGroupSelector
                selectedModel={config.model}
                models={models}
                onModelChange={(value) => updateConfig('model', value)}
                selectedGroup={config.group}
                groups={groups}
                onGroupChange={(value) => updateConfig('group', value)}
                disabled={isWorking}
              />

              {mode === 'synthesize' ? (
                <>
                  <div className='space-y-2'>
                    <Label htmlFor='speech-text'>{t('Text')}</Label>
                    <Textarea
                      id='speech-text'
                      onChange={(event) => setText(event.target.value)}
                      placeholder={t('Text to read aloud')}
                      rows={5}
                      value={text}
                    />
                  </div>

                  <div className='space-y-2'>
                    <Label htmlFor='speech-voice'>{t('Voice')}</Label>
                    <Input
                      id='speech-voice'
                      list='speech-voice-suggestions'
                      onChange={(event) =>
                        updateConfig('voice', event.target.value)
                      }
                      value={config.voice}
                    />
                    <datalist id='speech-voice-suggestions'>
                      {SPEECH_VOICE_SUGGESTIONS.map((voice) => (
                        <option key={voice} value={voice} />
                      ))}
                    </datalist>
                  </div>

                  <div className='grid grid-cols-2 gap-3'>
                    <div className='space-y-2'>
                      <Label htmlFor='speech-format'>{t('Format')}</Label>
                      <Select
                        onValueChange={(value) =>
                          updateConfig(
                            'responseFormat',
                            value ?? config.responseFormat
                          )
                        }
                        value={config.responseFormat}
                      >
                        <SelectTrigger id='speech-format'>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {SPEECH_FORMAT_OPTIONS.map((format) => (
                            <SelectItem key={format} value={format}>
                              {format}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className='space-y-2'>
                      <Label htmlFor='speech-speed'>{t('Speed')}</Label>
                      <Input
                        id='speech-speed'
                        max={SPEECH_SPEED_MAX}
                        min={SPEECH_SPEED_MIN}
                        onChange={(event) =>
                          updateConfig('speed', Number(event.target.value) || 1)
                        }
                        step={0.05}
                        type='number'
                        value={config.speed}
                      />
                    </div>
                  </div>
                </>
              ) : (
                <div className='space-y-2'>
                  <Label>{t('Audio file')}</Label>
                  <input
                    accept='audio/*'
                    className='hidden'
                    onChange={handleAudioFile}
                    ref={fileInputRef}
                    type='file'
                  />
                  <Button
                    className='w-full'
                    disabled={isWorking}
                    onClick={() => fileInputRef.current?.click()}
                    type='button'
                    variant='outline'
                  >
                    {file ? t('Replace audio file') : t('Choose audio file')}
                  </Button>
                  {file && (
                    <p className='text-muted-foreground truncate text-xs'>
                      {file.name}
                    </p>
                  )}
                </div>
              )}

              {error && (
                <Alert variant='destructive'>
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}

              <Button
                className='w-full'
                disabled={!canRun}
                onClick={run}
                type='button'
              >
                {isWorking && <Spinner />}
                {isWorking
                  ? t('Working')
                  : mode === 'synthesize'
                    ? t('Generate speech')
                    : t('Transcribe')}
              </Button>
            </CardContent>
          </Card>

          <div className='min-w-0'>
            {mode === 'synthesize' &&
              (!audioUrl ? (
                <div className='text-muted-foreground border-muted flex min-h-64 items-center justify-center rounded-xl border border-dashed p-8 text-center text-sm'>
                  {t('Generated speech will play here')}
                </div>
              ) : (
                <Card className='gap-0 py-0'>
                  <CardContent className='space-y-3 py-4'>
                    <audio className='w-full' controls src={audioUrl} />
                    <a
                      className='text-primary text-xs hover:underline'
                      download={`speech.${config.responseFormat}`}
                      href={audioUrl}
                    >
                      {t('Download')}
                    </a>
                  </CardContent>
                </Card>
              ))}

            {mode === 'transcribe' &&
              (!transcript ? (
                <div className='text-muted-foreground border-muted flex min-h-64 items-center justify-center rounded-xl border border-dashed p-8 text-center text-sm'>
                  {t('The transcript will appear here')}
                </div>
              ) : (
                <Card className='gap-0 py-0'>
                  <CardContent className='space-y-2 py-4'>
                    <p className='text-sm whitespace-pre-wrap'>{transcript}</p>
                  </CardContent>
                </Card>
              ))}
          </div>
        </div>
      </ScrollArea>
    </div>
  )
}
