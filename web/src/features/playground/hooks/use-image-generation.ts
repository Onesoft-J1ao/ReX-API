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
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { editImages, generateImages } from '../api'
import { DEFAULT_IMAGE_CONFIG, MAX_REFERENCE_IMAGES } from '../constants'
import type {
  GeneratedImage,
  ImageWorkbenchConfig,
  ImageWorkbenchMode,
} from '../types'

export function useImageGeneration() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<ImageWorkbenchConfig>({
    ...DEFAULT_IMAGE_CONFIG,
  })
  const [mode, setMode] = useState<ImageWorkbenchMode>('text-to-image')
  const [prompt, setPrompt] = useState('')
  const [referenceImages, setReferenceImages] = useState<File[]>([])
  const [mask, setMask] = useState<File | null>(null)
  const [images, setImages] = useState<GeneratedImage[]>([])
  const [isGenerating, setIsGenerating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const updateConfig = useCallback(
    <K extends keyof ImageWorkbenchConfig>(
      key: K,
      value: ImageWorkbenchConfig[K]
    ) => {
      setConfig((previous) => ({ ...previous, [key]: value }))
    },
    []
  )

  const addReferenceImages = useCallback((files: File[]) => {
    setReferenceImages((previous) =>
      [...previous, ...files].slice(0, MAX_REFERENCE_IMAGES)
    )
  }, [])

  const removeReferenceImage = useCallback((index: number) => {
    setReferenceImages((previous) =>
      previous.filter((_, position) => position !== index)
    )
  }, [])

  // Image-to-image needs at least one reference image; the provider rejects an
  // edit request without one, so the panel blocks it before spending quota.
  const canGenerate =
    !isGenerating &&
    prompt.trim().length > 0 &&
    config.model.length > 0 &&
    (mode === 'text-to-image' || referenceImages.length > 0)

  const generate = useCallback(async () => {
    if (!canGenerate) {
      return
    }
    setIsGenerating(true)
    setError(null)
    try {
      const response =
        mode === 'image-to-image'
          ? await editImages(config, prompt, referenceImages, mask)
          : await generateImages(config, prompt)
      const produced = response?.data ?? []
      setImages(produced)
      if (produced.length === 0) {
        setError(t('The provider returned no image'))
      }
    } catch (requestError) {
      const failure = requestError as {
        response?: { data?: { error?: { message?: string }; message?: string } }
        message?: string
      }
      setError(
        failure?.response?.data?.error?.message ??
          failure?.response?.data?.message ??
          failure?.message ??
          t('Request failed')
      )
    } finally {
      setIsGenerating(false)
    }
  }, [canGenerate, config, mask, mode, prompt, referenceImages, t])

  return {
    config,
    updateConfig,
    clearImages: () => setImages([]),
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
  }
}
