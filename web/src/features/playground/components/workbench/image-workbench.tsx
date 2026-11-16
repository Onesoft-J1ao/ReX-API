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
import { Copy, Download, ExternalLink, ImageIcon, Trash2 } from 'lucide-react'
import { useEffect, useRef, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelGroupSelector } from '@/components/model-group-selector'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
  IMAGE_N_MAX,
  IMAGE_QUALITY_OPTIONS,
  IMAGE_SIZE_OPTIONS,
  MAX_REFERENCE_IMAGES,
} from '../../constants'
import { useImageGeneration } from '../../hooks/use-image-generation'
import type { GeneratedImage } from '../../types'

const ASPECT_PRESETS = [
  { label: '1:1', size: '1024x1024' },
  { label: '3:2', size: '1536x1024' },
  { label: '2:3', size: '1024x1536' },
  { label: '16:9', size: '1792x1024' },
  { label: '9:16', size: '1024x1792' },
] as const

const INPUT_FIDELITY_OPTIONS = [
  { value: 'default', label: 'Provider default' },
  { value: 'low', label: 'Low' },
  { value: 'high', label: 'High' },
] as const

function imageSource(image: GeneratedImage): string {
  if (image.url) {
    return image.url
  }
  if (image.b64_json) {
    return `data:image/png;base64,${image.b64_json}`
  }
  return ''
}

export function ImageWorkbench() {
  const { t } = useTranslation()
  const referenceInputRef = useRef<HTMLInputElement>(null)
  const maskInputRef = useRef<HTMLInputElement>(null)
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
    mask,
    setMask,
    images,
    isGenerating,
    error,
    canGenerate,
    generate,
    clearImages,
  } = useImageGeneration()

  const { data: groups = [] } = useQuery({
    queryKey: ['playground', 'workbench', 'groups'],
    queryFn: getUserGroups,
  })
  const { data: models = [] } = useQuery({
    queryKey: ['playground', 'workbench', 'models', config.group],
    queryFn: () => getUserModels(config.group),
  })

  // The pickers start empty: fall back to the first group and model the account
  // can actually reach, so the panel is usable without touching the selectors.
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
    const picked = [...(event.target.files ?? [])]
    if (picked.length > 0) {
      addReferenceImages(picked)
    }
    event.target.value = ''
  }

  const handleMaskFile = (event: ChangeEvent<HTMLInputElement>) => {
    setMask(event.target.files?.[0] ?? null)
    event.target.value = ''
  }

  return (
    <div className='flex size-full min-h-0 flex-col overflow-hidden'>
      <ScrollArea className='min-h-0 flex-1'>
        <div className='mx-auto grid w-full max-w-6xl gap-4 p-4 sm:p-6 lg:grid-cols-[22rem_minmax(0,1fr)]'>
          <Card className='h-fit'>
            <CardHeader>
              <CardTitle>{t('Image generation')}</CardTitle>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='bg-muted flex rounded-md p-1'>
                <Button
                  className='flex-1'
                  onClick={() => setMode('text-to-image')}
                  size='sm'
                  variant={mode === 'text-to-image' ? 'secondary' : 'ghost'}
                >
                  {t('Text to image')}
                </Button>
                <Button
                  className='flex-1'
                  onClick={() => setMode('image-to-image')}
                  size='sm'
                  variant={mode === 'image-to-image' ? 'secondary' : 'ghost'}
                >
                  {t('Image to image')}
                </Button>
              </div>

              <ModelGroupSelector
                selectedModel={config.model}
                models={models}
                onModelChange={(value) => updateConfig('model', value)}
                selectedGroup={config.group}
                groups={groups}
                onGroupChange={(value) => updateConfig('group', value)}
                disabled={isGenerating}
              />

              <div className='space-y-2'>
                <Label htmlFor='image-prompt'>{t('Prompt')}</Label>
                <Textarea
                  id='image-prompt'
                  value={prompt}
                  onChange={(event) => setPrompt(event.target.value)}
                  onKeyDown={(event) => {
                    if (
                      (event.metaKey || event.ctrlKey) &&
                      event.key === 'Enter' &&
                      canGenerate
                    ) {
                      event.preventDefault()
                      void generate()
                    }
                  }}
                  placeholder={t('Describe the image you want to generate')}
                  rows={4}
                />
                <p className='text-muted-foreground text-[11px]'>
                  {t('Ctrl/⌘ + Enter to generate')}
                </p>
              </div>

              {mode === 'image-to-image' && (
                <div className='space-y-3'>
                  <div className='space-y-2'>
                    <Label>
                      {t('Reference images')}{' '}
                      <span className='text-muted-foreground'>
                        {referenceImages.length}/{MAX_REFERENCE_IMAGES}
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
                        isGenerating ||
                        referenceImages.length >= MAX_REFERENCE_IMAGES
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
                            <span className='truncate text-xs'>
                              {file.name}
                            </span>
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

                  <div className='space-y-2'>
                    <Label>{t('Mask')}</Label>
                    <input
                      ref={maskInputRef}
                      accept='image/*'
                      className='hidden'
                      onChange={handleMaskFile}
                      type='file'
                    />
                    <div className='flex items-center gap-2'>
                      <Button
                        className='flex-1'
                        disabled={isGenerating}
                        onClick={() => maskInputRef.current?.click()}
                        type='button'
                        variant='outline'
                      >
                        {mask ? t('Replace mask') : t('Add mask')}
                      </Button>
                      {mask && (
                        <Button
                          onClick={() => setMask(null)}
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
                    <Label htmlFor='image-input-fidelity'>
                      {t('Input fidelity')}
                    </Label>
                    <Select
                      value={config.inputFidelity || 'default'}
                      onValueChange={(value) =>
                        updateConfig(
                          'inputFidelity',
                          !value || value === 'default' ? '' : value
                        )
                      }
                    >
                      <SelectTrigger id='image-input-fidelity'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {INPUT_FIDELITY_OPTIONS.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {t(option.label)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              )}

              <div className='space-y-2'>
                <Label>{t('Aspect ratio')}</Label>
                <div className='flex flex-wrap gap-1.5'>
                  {ASPECT_PRESETS.map((preset) => (
                    <Button
                      className='h-7 px-2 font-mono text-xs'
                      key={preset.label}
                      onClick={() => updateConfig('size', preset.size)}
                      size='sm'
                      type='button'
                      variant={
                        config.size === preset.size ? 'secondary' : 'outline'
                      }
                    >
                      {preset.label}
                    </Button>
                  ))}
                </div>
              </div>

              <div className='grid grid-cols-2 gap-3'>
                <div className='space-y-2'>
                  <Label htmlFor='image-size'>{t('Size')}</Label>
                  <Select
                    value={config.size}
                    onValueChange={(value) =>
                      updateConfig('size', value ?? config.size)
                    }
                  >
                    <SelectTrigger id='image-size'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {IMAGE_SIZE_OPTIONS.map((size) => (
                        <SelectItem key={size} value={size}>
                          {size}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className='space-y-2'>
                  <Label htmlFor='image-quality'>{t('Quality')}</Label>
                  <Select
                    value={config.quality}
                    onValueChange={(value) =>
                      updateConfig('quality', value ?? config.quality)
                    }
                  >
                    <SelectTrigger id='image-quality'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {IMAGE_QUALITY_OPTIONS.map((quality) => (
                        <SelectItem key={quality} value={quality}>
                          {quality}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className='space-y-2'>
                <Label htmlFor='image-count'>{t('Number of images')}</Label>
                <Select
                  value={String(config.n)}
                  onValueChange={(value) =>
                    updateConfig('n', value ? Number(value) : config.n)
                  }
                >
                  <SelectTrigger id='image-count'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Array.from(
                      { length: IMAGE_N_MAX },
                      (_, index) => index + 1
                    ).map((count) => (
                      <SelectItem key={count} value={String(count)}>
                        {count}
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
                disabled={!canGenerate}
                onClick={generate}
                type='button'
              >
                {isGenerating && <Spinner />}
                {isGenerating ? t('Generating') : t('Generate')}
              </Button>
            </CardContent>
          </Card>

          <div className='min-w-0'>
            {images.length === 0 && !isGenerating && (
              <div className='text-muted-foreground border-muted flex min-h-64 flex-col items-center justify-center gap-3 rounded-xl border border-dashed p-8 text-center text-sm'>
                <ImageIcon className='h-8 w-8 opacity-60' />
                <span>{t('Generated images will appear here')}</span>
              </div>
            )}

            {images.length > 0 && (
              <div className='mb-3 flex items-center justify-between'>
                <Badge variant='secondary'>
                  {t('{{count}} images', { count: images.length })}
                </Badge>
                <Button
                  onClick={clearImages}
                  size='sm'
                  type='button'
                  variant='ghost'
                >
                  <Trash2 className='mr-1 h-3.5 w-3.5' />
                  {t('Clear')}
                </Button>
              </div>
            )}

            {isGenerating && (
              <div className='grid gap-4 sm:grid-cols-2'>
                {Array.from({ length: config.n }).map((_, index) => (
                  <div
                    className='bg-muted/40 flex aspect-square items-center justify-center rounded-xl border'
                    key={`pending-${index}`}
                  >
                    <Spinner />
                  </div>
                ))}
              </div>
            )}

            {images.length > 0 && (
              <div className='grid gap-4 sm:grid-cols-2'>
                {images.map((image, index) => {
                  const source = imageSource(image)
                  return (
                    <Card key={index} className='gap-0 overflow-hidden py-0'>
                      {source && (
                        <img
                          alt={image.revised_prompt ?? t('Generated image')}
                          className='w-full object-contain'
                          src={source}
                        />
                      )}
                      <CardContent className='flex items-center justify-between gap-2 py-3'>
                        <span className='text-muted-foreground min-w-0 flex-1 truncate text-xs'>
                          {image.revised_prompt ?? t('Generated image')}
                        </span>
                        {source && (
                          <div className='flex shrink-0 items-center gap-1'>
                            <Button
                              aria-label={t('Copy prompt')}
                              onClick={() =>
                                void navigator.clipboard.writeText(
                                  image.revised_prompt ?? prompt
                                )
                              }
                              size='icon'
                              type='button'
                              variant='ghost'
                            >
                              <Copy className='h-3.5 w-3.5' />
                            </Button>
                            <a
                              aria-label={t('Open in new tab')}
                              className='text-muted-foreground hover:text-foreground inline-flex h-8 w-8 items-center justify-center rounded-md'
                              href={source}
                              rel='noreferrer'
                              target='_blank'
                            >
                              <ExternalLink className='h-3.5 w-3.5' />
                            </a>
                            <a
                              aria-label={t('Download')}
                              className='text-muted-foreground hover:text-foreground inline-flex h-8 w-8 items-center justify-center rounded-md'
                              download={`image-${index + 1}.png`}
                              href={source}
                            >
                              <Download className='h-3.5 w-3.5' />
                            </a>
                          </div>
                        )}
                      </CardContent>
                    </Card>
                  )
                })}
              </div>
            )}
          </div>
        </div>
      </ScrollArea>
    </div>
  )
}
