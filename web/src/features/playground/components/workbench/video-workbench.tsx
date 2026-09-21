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
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
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
  VIDEO_DURATION_MAX,
  VIDEO_DURATION_MIN,
  VIDEO_MAX_REFERENCE_IMAGES,
} from '../../constants'
import { useVideoGeneration } from '../../hooks/use-video-generation'

// Labels are listed in src/i18n/static-keys.ts because they are selected at
// runtime rather than written as literals next to t().
const VIDEO_STATUS_LABELS: Record<string, string> = {
  NOT_START: 'Queued',
  SUBMITTED: 'Queued',
  QUEUED: 'Queued',
  IN_PROGRESS: 'Processing',
  SUCCESS: 'Completed',
  FAILURE: 'Failed',
}

function videoProgressValue(progress: string): number {
  const parsed = Number.parseInt(progress, 10)
  if (Number.isNaN(parsed)) {
    return 0
  }
  return Math.min(Math.max(parsed, 0), 100)
}

export function VideoWorkbench() {
  const { t } = useTranslation()
  const referenceInputRef = useRef<HTMLInputElement>(null)
  const firstFrameInputRef = useRef<HTMLInputElement>(null)
  const lastFrameInputRef = useRef<HTMLInputElement>(null)
  const {
    config,
    updateConfig,
    mode,
    setMode,
    prompt,
    setPrompt,
    referenceImages,
    addReferenceImages,
    removeReferenceImage,
    firstFrame,
    setFirstFrame,
    lastFrame,
    setLastFrame,
    task,
    isSubmitting,
    isSettled,
    error,
    canRun,
    run,
  } = useVideoGeneration()

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

  const handleReferenceFiles = (event: ChangeEvent<HTMLInputElement>) => {
    const picked = Array.from(event.target.files ?? [])
    if (picked.length > 0) {
      addReferenceImages(picked)
    }
    event.target.value = ''
  }

  const handleFirstFrameFile = (event: ChangeEvent<HTMLInputElement>) => {
    setFirstFrame(event.target.files?.[0] ?? null)
    event.target.value = ''
  }

  const handleLastFrameFile = (event: ChangeEvent<HTMLInputElement>) => {
    setLastFrame(event.target.files?.[0] ?? null)
    event.target.value = ''
  }

  const isCompleted = task?.status === 'SUCCESS'
  const isFailed = task?.status === 'FAILURE'
  const isRunning = task !== null && !isSettled

  return (
    <div className='flex size-full min-h-0 flex-col overflow-hidden'>
      <ScrollArea className='min-h-0 flex-1'>
        <div className='mx-auto grid w-full max-w-6xl gap-4 p-4 sm:p-6 lg:grid-cols-[22rem_minmax(0,1fr)]'>
          <Card className='h-fit'>
            <CardHeader>
              <CardTitle>{t('Video generation')}</CardTitle>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='bg-muted flex rounded-md p-1'>
                <Button
                  className='flex-1'
                  onClick={() => setMode('text')}
                  size='sm'
                  variant={mode === 'text' ? 'secondary' : 'ghost'}
                >
                  {t('Text to video')}
                </Button>
                <Button
                  className='flex-1'
                  onClick={() => setMode('reference')}
                  size='sm'
                  variant={mode === 'reference' ? 'secondary' : 'ghost'}
                >
                  {t('Reference images')}
                </Button>
                <Button
                  className='flex-1'
                  onClick={() => setMode('keyframe')}
                  size='sm'
                  variant={mode === 'keyframe' ? 'secondary' : 'ghost'}
                >
                  {t('Keyframes')}
                </Button>
              </div>

              <ModelGroupSelector
                selectedModel={config.model}
                models={models}
                onModelChange={(value) => updateConfig('model', value)}
                selectedGroup={config.group}
                groups={groups}
                onGroupChange={(value) => updateConfig('group', value)}
                disabled={isSubmitting}
              />

              <div className='space-y-2'>
                <Label htmlFor='video-prompt'>{t('Prompt')}</Label>
                <Textarea
                  id='video-prompt'
                  value={prompt}
                  onChange={(event) => setPrompt(event.target.value)}
                  placeholder={t('Describe the video you want to generate')}
                  rows={4}
                />
              </div>

              {mode === 'reference' && (
                <div className='space-y-2'>
                  <Label>
                    {t('Reference images')}{' '}
                    <span className='text-muted-foreground'>
                      {referenceImages.length}/{VIDEO_MAX_REFERENCE_IMAGES}
                    </span>
                  </Label>
                  <input
                    ref={referenceInputRef}
                    accept='image/*'
                    className='hidden'
                    multiple
                    onChange={handleReferenceFiles}
                    type='file'
                  />
                  <Button
                    className='w-full'
                    disabled={
                      isSubmitting ||
                      referenceImages.length >= VIDEO_MAX_REFERENCE_IMAGES
                    }
                    onClick={() => referenceInputRef.current?.click()}
                    type='button'
                    variant='outline'
                  >
                    {t('Add reference image')}
                  </Button>
                  {referenceImages.length > 0 && (
                    <ul className='space-y-1'>
                      {referenceImages.map((file, index) => (
                        <li
                          key={`${file.name}-${index}`}
                          className='bg-muted flex items-center justify-between gap-2 rounded-md px-2 py-1'
                        >
                          <span className='truncate text-xs'>{file.name}</span>
                          <Button
                            onClick={() => removeReferenceImage(index)}
                            size='sm'
                            type='button'
                            variant='ghost'
                          >
                            {t('Remove')}
                          </Button>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              )}

              {mode === 'keyframe' && (
                <div className='space-y-3'>
                  <div className='space-y-2'>
                    <Label>{t('Start frame')}</Label>
                    <input
                      ref={firstFrameInputRef}
                      accept='image/*'
                      className='hidden'
                      onChange={handleFirstFrameFile}
                      type='file'
                    />
                    <div className='flex items-center gap-2'>
                      <Button
                        className='flex-1'
                        disabled={isSubmitting}
                        onClick={() => firstFrameInputRef.current?.click()}
                        type='button'
                        variant='outline'
                      >
                        {firstFrame
                          ? t('Replace start frame')
                          : t('Add start frame')}
                      </Button>
                      {firstFrame && (
                        <Button
                          onClick={() => setFirstFrame(null)}
                          size='sm'
                          type='button'
                          variant='ghost'
                        >
                          {t('Remove')}
                        </Button>
                      )}
                    </div>
                  </div>

                  <div className='space-y-2'>
                    <Label>{t('End frame')}</Label>
                    <input
                      ref={lastFrameInputRef}
                      accept='image/*'
                      className='hidden'
                      onChange={handleLastFrameFile}
                      type='file'
                    />
                    <div className='flex items-center gap-2'>
                      <Button
                        className='flex-1'
                        disabled={isSubmitting}
                        onClick={() => lastFrameInputRef.current?.click()}
                        type='button'
                        variant='outline'
                      >
                        {lastFrame
                          ? t('Replace end frame')
                          : t('Add end frame')}
                      </Button>
                      {lastFrame && (
                        <Button
                          onClick={() => setLastFrame(null)}
                          size='sm'
                          type='button'
                          variant='ghost'
                        >
                          {t('Remove')}
                        </Button>
                      )}
                    </div>
                  </div>
                </div>
              )}

              <div className='space-y-2'>
                <Label htmlFor='video-duration'>{t('Duration')}</Label>
                <Select
                  value={String(config.duration)}
                  onValueChange={(value) =>
                    updateConfig('duration', value ? Number(value) : 0)
                  }
                >
                  <SelectTrigger id='video-duration'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Array.from(
                      { length: VIDEO_DURATION_MAX - VIDEO_DURATION_MIN + 1 },
                      (_, index) => VIDEO_DURATION_MIN + index
                    ).map((seconds) => (
                      <SelectItem key={seconds} value={String(seconds)}>
                        {t('{{count}}s', { count: seconds })}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

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
                {isSubmitting && <Spinner />}
                {isSubmitting ? t('Generating') : t('Generate video')}
              </Button>
            </CardContent>
          </Card>

          <div className='min-w-0'>
            {!task && !isSubmitting && (
              <div className='text-muted-foreground border-muted flex min-h-64 items-center justify-center rounded-xl border border-dashed p-8 text-center text-sm'>
                {t('Your video will appear here')}
              </div>
            )}

            {task && isRunning && (
              <Card className='gap-0 py-0'>
                <CardContent className='space-y-3 py-4'>
                  <div className='text-muted-foreground flex items-center gap-2 text-sm'>
                    <Spinner />
                    {t(VIDEO_STATUS_LABELS[task.status] ?? 'Processing')}
                  </div>
                  <Progress value={videoProgressValue(task.progress)} />
                </CardContent>
              </Card>
            )}

            {task && isFailed && (
              <Alert variant='destructive'>
                <AlertDescription>
                  {task.failReason || t('The provider returned no video')}
                </AlertDescription>
              </Alert>
            )}

            {task && isCompleted && (
              <Card className='gap-0 py-0'>
                <CardContent className='space-y-3 py-4'>
                  {task.resultUrl ? (
                    <>
                      <video
                        className='w-full rounded-md'
                        controls
                        src={task.resultUrl}
                      >
                        {t('Your video will appear here')}
                      </video>
                      <a
                        className='text-primary text-xs hover:underline'
                        download={`${task.taskId}.mp4`}
                        href={task.resultUrl}
                      >
                        {t('Download')}
                      </a>
                    </>
                  ) : (
                    <p className='text-muted-foreground text-sm'>
                      {t('The provider returned no video')}
                    </p>
                  )}
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      </ScrollArea>
    </div>
  )
}
