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

import { fetchThreeDTask, submitThreeDTask } from '../api'
import {
  DEFAULT_THREE_D_CONFIG,
  TASK_POLL_INTERVAL_MS,
  THREE_D_TEXTURE_MODEL_SUFFIX,
} from '../constants'
import { readFileAsDataUrl } from '../lib/media/file-to-data-url'
import type { ThreeDTask, ThreeDWorkbenchConfig } from '../types'

// Meshy reports lower case task states on both submit and fetch.
const TERMINAL_THREE_D_STATUSES = new Set(['completed', 'failed'])

function isThreeDTaskSettled(status: string): boolean {
  return TERMINAL_THREE_D_STATUSES.has(status)
}

export function isTextureModel(model: string): boolean {
  return model.endsWith(THREE_D_TEXTURE_MODEL_SUFFIX)
}

export function useThreeDGeneration() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<ThreeDWorkbenchConfig>({
    ...DEFAULT_THREE_D_CONFIG,
  })
  const [prompt, setPrompt] = useState('')
  const [inputReference, setInputReference] = useState<File | null>(null)
  const [sourceTaskId, setSourceTaskId] = useState('')
  const [task, setTask] = useState<ThreeDTask | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const updateConfig = useCallback(
    <K extends keyof ThreeDWorkbenchConfig>(
      key: K,
      value: ThreeDWorkbenchConfig[K]
    ) => {
      setConfig((previous) => ({ ...previous, [key]: value }))
    },
    []
  )

  // Texture models re-texture a finished draft and reject a prompt, while the
  // other models require a prompt or a reference image.
  const canRun =
    !isSubmitting &&
    config.model.length > 0 &&
    (isTextureModel(config.model)
      ? sourceTaskId.trim().length > 0
      : prompt.trim().length > 0 || inputReference !== null)

  const taskId = task?.taskId ?? ''
  const isSettled = task !== null && isThreeDTaskSettled(task.status)

  const refresh = useCallback(async () => {
    if (!taskId) {
      return
    }
    try {
      const response = await fetchThreeDTask(taskId)
      setTask({
        taskId,
        status: response?.status ?? '',
        progress: response?.progress ?? 0,
        format: response?.data?.format ?? '',
        url: response?.data?.url ?? '',
        failReason: response?.error?.message ?? '',
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
      const referenceDataUrl = inputReference
        ? await readFileAsDataUrl(inputReference)
        : ''
      const response = await submitThreeDTask(
        config,
        isTextureModel(config.model) ? '' : prompt,
        isTextureModel(config.model) ? '' : referenceDataUrl,
        isTextureModel(config.model) ? sourceTaskId.trim() : ''
      )
      const submittedId = response?.id ?? ''
      if (!submittedId) {
        setError(t('The provider returned no 3D model'))
        return
      }
      setTask({
        taskId: submittedId,
        status: response?.status ?? 'queued',
        progress: response?.progress ?? 0,
        format: response?.data?.format ?? '',
        url: response?.data?.url ?? '',
        failReason: response?.error?.message ?? '',
      })
    } catch (requestError) {
      setError(getServerErrorMessage(requestError, t('Request failed')))
    } finally {
      setIsSubmitting(false)
    }
  }, [canRun, config, inputReference, prompt, sourceTaskId, t])

  return {
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
  }
}
