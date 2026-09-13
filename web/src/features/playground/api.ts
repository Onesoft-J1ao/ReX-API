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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import { API_ENDPOINTS } from './constants'
import {
  getPlaygroundFingerprint,
  PLAYGROUND_FINGERPRINT_HEADER,
} from './lib/fingerprint'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ImageGenerationResponse,
  ImageWorkbenchConfig,
  ModelOption,
  GroupOption,
} from './types'

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest,
  signal?: AbortSignal
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    signal,
    skipErrorHandler: true,
    headers: { [PLAYGROUND_FINGERPRINT_HEADER]: getPlaygroundFingerprint() },
  } as Record<string, unknown>)
  return res.data
}

/**
 * Generate images from a text prompt. The wire shape of the image endpoints is
 * built here so callers pass workbench values, not raw payloads.
 */
export async function generateImages(
  config: ImageWorkbenchConfig,
  prompt: string,
  signal?: AbortSignal
): Promise<ImageGenerationResponse> {
  const res = await api.post(
    API_ENDPOINTS.IMAGE_GENERATIONS,
    {
      model: config.model,
      group: config.group,
      prompt,
      n: config.n,
      size: config.size,
      quality: config.quality,
    },
    {
      signal,
      skipErrorHandler: true,
      headers: { [PLAYGROUND_FINGERPRINT_HEADER]: getPlaygroundFingerprint() },
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Generate images from a prompt plus reference images. The edits endpoint speaks
 * multipart, so reference images and the optional mask are appended as files;
 * axios sets the multipart boundary itself, so no content type is passed here.
 */
export async function editImages(
  config: ImageWorkbenchConfig,
  prompt: string,
  images: File[],
  mask: File | null,
  signal?: AbortSignal
): Promise<ImageGenerationResponse> {
  const formData = new FormData()
  formData.append('model', config.model)
  if (config.group) {
    formData.append('group', config.group)
  }
  formData.append('prompt', prompt)
  formData.append('n', String(config.n))
  formData.append('size', config.size)
  formData.append('quality', config.quality)
  if (config.inputFidelity) {
    formData.append('input_fidelity', config.inputFidelity)
  }
  for (const image of images) {
    formData.append('image[]', image)
  }
  if (mask) {
    formData.append('mask', mask)
  }
  const res = await api.post(API_ENDPOINTS.IMAGE_EDITS, formData, {
    signal,
    skipErrorHandler: true,
    headers: { [PLAYGROUND_FINGERPRINT_HEADER]: getPlaygroundFingerprint() },
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(group: string): Promise<ModelOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_MODELS, {
    params: { group },
  })
  const { data } = res
  requireServerSuccess(data)

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  return data.data.map((model: string) => ({
    label: model,
    value: model,
  }))
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_GROUPS)
  const { data } = res
  requireServerSuccess(data)

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: info.ratio,
    desc: info.desc,
  }))
}
