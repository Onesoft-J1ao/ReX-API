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
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { getServerErrorMessage } from '@/lib/server-error-message'

import { fetchVideoTask, submitVideoTask } from '../api'
import {
  DEFAULT_VIDEO_CONFIG,
  TASK_POLL_INTERVAL_MS,
  VIDEO_MAX_REFERENCE_IMAGES,
} from '../constants'
import { readFileAsDataUrl } from '../lib/media/file-to-data-url'
import type {
  VideoTask,
  VideoWorkbenchConfig,
  VideoWorkbenchMode,
} from '../types'

// The gateway reports its own task states, which are upper case.
const TERMINAL_VIDEO_STATUSES = new Set(['SUCCESS', 'FAILURE'])

function isVideoTaskSettled(status: string): boolean {
  return TERMINAL_VIDEO_STATUSES.has(status)
}

export function useVideoGeneration() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<VideoWorkbenchConfig>({
    ...DEFAULT_VIDEO_CONFIG,
  })
  const [mode, setMode] = useState<VideoWorkbenchMode>('text')
  const [prompt, setPrompt] = useState('')
  const [referenceImages, setReferenceImages] = useState<File[]>([])
  const [firstFrame, setFirstFrame] = useState<File | null>(null)
  const [lastFrame, setLastFrame] = useState<File | null>(null)
  const [task, setTask] = useState<VideoTask | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const updateConfig = useCallback(
    <K extends keyof VideoWorkbenchConfig>(
      key: K,
      value: VideoWorkbenchConfig[K]
    ) => {
      setConfig((previous) => ({ ...previous, [key]: value }))
    },
    []
  )

  const addReferenceImages = useCallback((files: File[]) => {
    setReferenceImages((previous) =>
      [...previous, ...files].slice(0, VIDEO_MAX_REFERENCE_IMAGES)
    )
  }, [])

  const removeReferenceImage = useCallback((index: number) => {
    setReferenceImages((previous) =>
      previous.filter((_, position) => position !== index)
    )
  }, [])

  // Every Agnes generation requires a prompt; the provider mode then decides
  // which images the request may carry, so the panel blocks a mode that has
  // none selected before it spends quota.
  const canRun =
    !isSubmitting &&
    config.model.length > 0 &&
    prompt.trim().length > 0 &&
    (mode === 'text' ||
      (mode === 'reference' && referenceImages.length > 0) ||
      (mode === 'keyframe' && Boolean(firstFrame || lastFrame)))

  const taskId = task?.taskId ?? ''
  const isSettled = task !== null && isVideoTaskSettled(task.status)

  const refresh = useCallback(async () => {
    if (!taskId) {
      return
    }
    try {
      const response = await fetchVideoTask(taskId)
      const data = response?.data
      setTask({
        taskId,
        status: data?.status ?? '',
        progress: data?.progress ?? '',
        resultUrl: data?.result_url ?? '',
        failReason: data?.fail_reason ?? '',
      })
    } catch (requestError) {
      setError(getServerErrorMessage(requestError, t('Request failed')))
    }
  }, [t, taskId])

  // The task APIs are asynchronous, so the panel keeps reading the task back
  // until it settles. Each update re-arms the timer and unmounting clears it.
  useEffect(() => {
    if (!task || isSettled) {
      return
    }
    const timer = setTimeout(() => {
      void refresh()
    }, TASK_POLL_INTERVAL_MS)
    return () => clearTimeout(timer)
  }, [isSettled, refresh, task])

  const run = useCallback(async () => {
    if (!canRun) {
      return
    }
    setIsSubmitting(true)
    setError(null)
    try {
      const [referenceDataUrls, firstFrameDataUrl, lastFrameDataUrl] =
        await Promise.all([
          Promise.all(referenceImages.map(readFileAsDataUrl)),
          firstFrame ? readFileAsDataUrl(firstFrame) : Promise.resolve(''),
          lastFrame ? readFileAsDataUrl(lastFrame) : Promise.resolve(''),
        ])
      const response = await submitVideoTask(config, {
        mode,
        prompt,
        referenceImages: referenceDataUrls,
        firstFrame: firstFrameDataUrl,
        lastFrame: lastFrameDataUrl,
      })
      const submittedId = response?.id ?? response?.task_id ?? ''
      if (!submittedId) {
        setError(t('The provider returned no video'))
        return
      }
      setTask({
        taskId: submittedId,
        status: 'SUBMITTED',
        progress: '',
        resultUrl: '',
        failReason: '',
      })
    } catch (requestError) {
      setError(getServerErrorMessage(requestError, t('Request failed')))
    } finally {
      setIsSubmitting(false)
    }
  }, [canRun, config, firstFrame, lastFrame, mode, prompt, referenceImages, t])

  return {
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
  }
}
