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
import { THREE_D_ART_STYLE_OPTIONS } from '../../constants'
import {
  isTextureModel,
  useThreeDGeneration,
} from '../../hooks/use-three-d-generation'

// Labels are listed in src/i18n/static-keys.ts because they are selected at
// runtime rather than written as literals next to t().
const THREE_D_STATUS_LABELS: Record<string, string> = {
  queued: 'Queued',
  in_progress: 'Processing',
  completed: 'Completed',
  failed: 'Failed',
}

export function ThreeDWorkbench() {
  const { t } = useTranslation()
  const referenceInputRef = useRef<HTMLInputElement>(null)
  const {
    config,
    updateConfig,
    prompt,
    setPrompt,
    inputReference,
    setInputReference,
    sourceTaskId,
    setSourceTaskId,
    task,
    isSubmitting,
    isSettled,
    error,
    canRun,
    run,
  } = useThreeDGeneration()

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

  const handleReferenceFile = (event: ChangeEvent<HTMLInputElement>) => {
    setInputReference(event.target.files?.[0] ?? null)
    event.target.value = ''
  }

  const isTexture = isTextureModel(config.model)
  const isCompleted = task?.status === 'completed'
  const isFailed = task?.status === 'failed'
  const isRunning = task !== null && !isSettled

  return (
    <div className='flex size-full min-h-0 flex-col overflow-hidden'>
      <ScrollArea className='min-h-0 flex-1'>
        <div className='mx-auto grid w-full max-w-6xl gap-4 p-4 sm:p-6 lg:grid-cols-[22rem_minmax(0,1fr)]'>
          <Card className='h-fit'>
            <CardHeader>
              <CardTitle>{t('3D generation')}</CardTitle>
            </CardHeader>
            <CardContent className='space-y-4'>
              <ModelGroupSelector
                selectedModel={config.model}
                models={models}
                onModelChange={(value) => updateConfig('model', value)}
                selectedGroup={config.group}
                groups={groups}
                onGroupChange={(value) => updateConfig('group', value)}
                disabled={isSubmitting}
              />

              {isTexture ? (
                <div className='space-y-2'>
                  <Label htmlFor='three-d-source-task'>
                    {t('Source task ID')}
                  </Label>
                  <Input
                    id='three-d-source-task'
                    onChange={(event) => setSourceTaskId(event.target.value)}
                    value={sourceTaskId}
                  />
                  <p className='text-muted-foreground text-xs'>
                    {t('Reuse a completed draft task to add textures.')}
                  </p>
                </div>
              ) : (
                <>
                  <div className='space-y-2'>
                    <Label htmlFor='three-d-prompt'>{t('Prompt')}</Label>
                    <Textarea
                      id='three-d-prompt'
                      onChange={(event) => setPrompt(event.target.value)}
                      placeholder={t(
                        'Describe the 3D model you want to generate'
                      )}
                      rows={4}
                      value={prompt}
                    />
                  </div>

                  <div className='space-y-2'>
                    <Label>{t('Reference image')}</Label>
                    <input
                      accept='image/*'
                      className='hidden'
                      onChange={handleReferenceFile}
                      ref={referenceInputRef}
                      type='file'
                    />
                    <Button
                      className='w-full'
                      disabled={isSubmitting}
                      onClick={() => referenceInputRef.current?.click()}
                      type='button'
                      variant='outline'
                    >
                      {t('Add reference image')}
                    </Button>
                    {inputReference && (
                      <div className='flex items-center gap-2'>
                        <span className='truncate text-xs'>
                          {inputReference.name}
                        </span>
                        <Button
                          onClick={() => setInputReference(null)}
                          size='sm'
                          type='button'
                          variant='ghost'
                        >
                          {t('Remove')}
                        </Button>
                      </div>
                    )}
                  </div>
                </>
              )}

              <div className='space-y-2'>
                <Label htmlFor='three-d-art-style'>{t('Art style')}</Label>
                <Select
                  onValueChange={(value) =>
                    updateConfig(
                      'artStyle',
                      !value || value === 'default' ? '' : value
                    )
                  }
                  value={config.artStyle || 'default'}
                >
                  <SelectTrigger id='three-d-art-style'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {THREE_D_ART_STYLE_OPTIONS.map((option) => (
                      <SelectItem
                        key={option.value || 'default'}
                        value={option.value || 'default'}
                      >
                        {t(option.label)}
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
                {isSubmitting ? t('Generating') : t('Generate 3D model')}
              </Button>
            </CardContent>
          </Card>

          <div className='min-w-0'>
            {!task && !isSubmitting && (
              <div className='text-muted-foreground border-muted flex min-h-64 items-center justify-center rounded-xl border border-dashed p-8 text-center text-sm'>
                {t('Your 3D model will appear here')}
              </div>
            )}

            {task && isRunning && (
              <Card className='gap-0 py-0'>
                <CardContent className='space-y-3 py-4'>
                  <div className='text-muted-foreground flex items-center gap-2 text-sm'>
                    <Spinner />
                    {t(THREE_D_STATUS_LABELS[task.status] ?? 'Processing')}
                  </div>
                  <Progress value={task.progress} />
                </CardContent>
              </Card>
            )}

            {task && isFailed && (
              <Alert variant='destructive'>
                <AlertDescription>
                  {task.failReason || t('The provider returned no 3D model')}
                </AlertDescription>
              </Alert>
            )}

            {task && isCompleted && (
              <Card className='gap-0 py-0'>
                <CardContent className='space-y-3 py-4'>
                  {task.url ? (
                    <a
                      className='text-primary text-xs hover:underline'
                      download={`${task.taskId}.${task.format || 'glb'}`}
                      href={task.url}
                    >
                      {t('Download')}
                    </a>
                  ) : (
                    <p className='text-muted-foreground text-sm'>
                      {t('The provider returned no 3D model')}
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
