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
  ImageEditRequest,
  ImageGenerationRequest,
  ImageGenerationResponse,
  ImageWorkbenchConfig,
  ModelOption,
  GroupOption,
  SpeechGenerationRequest,
  SpeechGenerationTaskResponse,
  SpeechWorkbenchConfig,
  ThreeDGenerationRequest,
  ThreeDGenerationResponse,
  TranscriptionResponse,
  VideoGenerationRequest,
  VideoGenerationResponse,
  ThreeDTaskResponse,
  ThreeDWorkbenchConfig,
  VideoTaskFetchResponse,
  VideoTaskInput,
  VideoTaskSubmitResponse,
  VideoWorkbenchConfig,
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
      background: config.background,
      output_format: config.outputFormat,
      moderation: config.moderation,
      output_compression: config.compression,
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
  if (config.background) {
    formData.append('background', config.background)
  }
  if (config.outputFormat) {
    formData.append('output_format', config.outputFormat)
  }
  if (config.moderation) {
    formData.append('moderation', config.moderation)
  }
  formData.append('output_compression', String(config.compression))
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

// ---------------------------------------------------------------------------
// Third-party OpenAI-compatible endpoints
// ---------------------------------------------------------------------------

/** Accepts `https://host`, `https://host/v1` and trims trailing slashes. */
function thirdPartyBaseUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim().replace(/\/+$/, '')
  if (!trimmed) return ''
  return /\/v\d+$/.test(trimmed) ? trimmed : `${trimmed}/v1`
}

async function readJsonOrThrow(
  response: Response
): Promise<ImageGenerationResponse> {
  const raw = await response.text()
  let payload: unknown = null
  try {
    payload = raw ? JSON.parse(raw) : null
  } catch {
    payload = null
  }
  if (!response.ok) {
    const failure = payload as {
      error?: { message?: string }
      message?: string
    } | null
    throw new Error(
      failure?.error?.message ??
        failure?.message ??
        `HTTP ${response.status} ${response.statusText}`
    )
  }
  return (payload as ImageGenerationResponse) ?? { data: [] }
}

function thirdPartyFields(config: ImageWorkbenchConfig) {
  return {
    background: config.background,
    output_format: config.outputFormat,
    moderation: config.moderation,
    output_compression: config.compression,
  }
}

/**
 * Call a third-party OpenAI-compatible /images/generations directly, bypassing
 * this gateway. The key stays in memory: it is used for this fetch only.
 */
export async function generateImagesThirdParty(
  config: ImageWorkbenchConfig,
  prompt: string,
  signal?: AbortSignal
): Promise<ImageGenerationResponse> {
  const base = thirdPartyBaseUrl(config.baseUrl)
  if (!base) throw new Error('Base URL is required')
  const response = await fetch(`${base}/images/generations`, {
    method: 'POST',
    signal,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${config.apiKey}`,
    },
    body: JSON.stringify({
      model: config.model,
      prompt,
      n: config.n,
      size: config.size,
      quality: config.quality,
      ...thirdPartyFields(config),
    }),
  })
  return readJsonOrThrow(response)
}

/** Same as above for the multipart /images/edits endpoint. */
export async function editImagesThirdParty(
  config: ImageWorkbenchConfig,
  prompt: string,
  images: File[],
  mask: File | null,
  signal?: AbortSignal
): Promise<ImageGenerationResponse> {
  const base = thirdPartyBaseUrl(config.baseUrl)
  if (!base) throw new Error('Base URL is required')
  const formData = new FormData()
  formData.append('model', config.model)
  formData.append('prompt', prompt)
  formData.append('n', String(config.n))
  formData.append('size', config.size)
  formData.append('quality', config.quality)
  for (const image of images) {
    formData.append('image[]', image)
  }
  if (mask) {
    formData.append('mask', mask)
  }
  const fields = thirdPartyFields(config)
  formData.append('background', fields.background)
  formData.append('output_format', fields.output_format)
  formData.append('moderation', fields.moderation)
  formData.append('output_compression', String(fields.output_compression))
  const response = await fetch(`${base}/images/edits`, {
    method: 'POST',
    signal,
    headers: { Authorization: `Bearer ${config.apiKey}` },
    body: formData,
  })
  return readJsonOrThrow(response)
}

/**
 * Synthesize speech. The endpoint answers with audio bytes rather than JSON, so
 * the response is read as a blob.
 */
export async function synthesizeSpeech(
  config: SpeechWorkbenchConfig,
  text: string,
  signal?: AbortSignal
): Promise<Blob> {
  const res = await api.post(
    API_ENDPOINTS.AUDIO_SPEECH,
    {
      model: config.model,
      group: config.group,
      input: text,
      voice: config.voice,
      response_format: config.responseFormat,
      speed: config.speed,
    },
    {
      signal,
      skipErrorHandler: true,
      responseType: 'blob',
      headers: { [PLAYGROUND_FINGERPRINT_HEADER]: getPlaygroundFingerprint() },
    } as Record<string, unknown>
  )
  return res.data as Blob
}

/**
 * Transcribe an audio file. The endpoint expects multipart, so the file is
 * appended as form data.
 */
export async function transcribeAudio(
  config: SpeechWorkbenchConfig,
  file: File,
  signal?: AbortSignal
): Promise<TranscriptionResponse> {
  const formData = new FormData()
  formData.append('model', config.model)
  if (config.group) {
    formData.append('group', config.group)
  }
  formData.append('file', file)
  const res = await api.post(API_ENDPOINTS.AUDIO_TRANSCRIPTIONS, formData, {
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

  return (data.data as string[])
    .map((model: string) => ({
      label: model,
      value: model,
    }))
    .sort((a, b) =>
      a.label.localeCompare(b.label, undefined, {
        numeric: true,
        sensitivity: 'base',
      })
    )
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

export async function generateImage(
  payload: ImageGenerationRequest,
  signal?: AbortSignal
): Promise<ImageGenerationResponse> {
  const res = await api.post(API_ENDPOINTS.IMAGE_GENERATIONS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function editImage(
  payload: ImageEditRequest,
  signal?: AbortSignal
): Promise<ImageGenerationResponse> {
  const res = await api.post(API_ENDPOINTS.IMAGE_EDITS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function generateSpeech(
  payload: SpeechGenerationRequest,
  signal?: AbortSignal
): Promise<Blob> {
  const res = await api.post(API_ENDPOINTS.SPEECH, payload, {
    signal,
    responseType: 'blob',
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data as Blob
}

export async function generateSpeechTask(
  payload: SpeechGenerationRequest,
  signal?: AbortSignal
): Promise<SpeechGenerationTaskResponse> {
  const res = await api.post(API_ENDPOINTS.SPEECH_TASKS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function getSpeechTask(
  taskId: string,
  signal?: AbortSignal
): Promise<SpeechGenerationTaskResponse> {
  const res = await api.get(`${API_ENDPOINTS.SPEECH_TASKS}/${taskId}`, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function getSpeechTaskContent(
  taskId: string,
  signal?: AbortSignal
): Promise<Blob> {
  const res = await api.get(`${API_ENDPOINTS.SPEECH_TASKS}/${taskId}/content`, {
    signal,
    responseType: 'blob',
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data as Blob
}

export async function generateThreeD(
  payload: ThreeDGenerationRequest
): Promise<ThreeDGenerationResponse> {
  const res = await api.post(API_ENDPOINTS.THREE_D, payload, {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function getThreeDTask(
  taskId: string,
  signal?: AbortSignal
): Promise<ThreeDGenerationResponse> {
  const res = await api.get(`${API_ENDPOINTS.THREE_D}/${taskId}`, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function generateVideo(
  payload: VideoGenerationRequest,
  signal?: AbortSignal
): Promise<VideoGenerationResponse> {
  const res = await api.post(API_ENDPOINTS.VIDEOS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function getVideoTask(
  taskId: string,
  signal?: AbortSignal
): Promise<VideoGenerationResponse> {
  const res = await api.get(`${API_ENDPOINTS.VIDEOS}/${taskId}`, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return normalizeVideoTaskResponse(res.data)
}

type RecordValue = Record<string, unknown>

function isRecord(value: unknown): value is RecordValue {
  return typeof value === 'object' && value !== null
}

const videoTaskStatusMap: Record<string, VideoGenerationResponse['status']> = {
  NOT_START: 'queued',
  SUBMITTED: 'queued',
  QUEUED: 'queued',
  IN_PROGRESS: 'in_progress',
  SUCCESS: 'completed',
  FAILURE: 'failed',
}

export function normalizeVideoTaskResponse(
  response: unknown
): VideoGenerationResponse {
  const envelope = isRecord(response) ? response : {}
  const raw =
    (envelope.success === true || envelope.code === 'success') &&
    isRecord(envelope.data)
      ? envelope.data
      : envelope
  const properties = isRecord(raw.properties) ? raw.properties : {}
  const rawStatus = String(raw.status ?? '').toUpperCase()
  const status = videoTaskStatusMap[rawStatus] ?? 'in_progress'
  const progressValue = Number.parseInt(String(raw.progress ?? '0'), 10)
  let resultURL: string | undefined
  if (typeof raw.result_url === 'string') {
    resultURL = raw.result_url
  } else if (isRecord(raw.data) && typeof raw.data.url === 'string') {
    resultURL = raw.data.url
  }
  const failReason = typeof raw.fail_reason === 'string' ? raw.fail_reason : ''

  return {
    id: String(raw.task_id ?? raw.id ?? ''),
    object: 'video',
    model: String(raw.model ?? properties.origin_model_name ?? ''),
    status,
    progress: Number.isFinite(progressValue) ? progressValue : 0,
    created_at: Number(raw.created_at ?? 0),
    ...(raw.finish_time
      ? { completed_at: Number(raw.finish_time) }
      : undefined),
    ...(resultURL ? { data: { url: resultURL } } : undefined),
    ...(failReason
      ? { error: { message: failReason, code: 'task_failed' } }
      : undefined),
  }
}

export async function submitVideoTask(
  config: VideoWorkbenchConfig,
  input: VideoTaskInput,
  signal?: AbortSignal
): Promise<VideoTaskSubmitResponse> {
  const payload: Record<string, unknown> = {
    model: config.model,
    group: config.group,
    prompt: input.prompt,
    mode: input.mode,
    duration: config.duration,
  }
  if (input.mode === 'reference') {
    payload.images = input.referenceImages
  }
  if (input.mode === 'keyframe') {
    if (input.firstFrame) {
      payload.first_frame = input.firstFrame
    }
    if (input.lastFrame) {
      payload.last_frame = input.lastFrame
    }
  }
  const res = await api.post(API_ENDPOINTS.VIDEO_GENERATIONS, payload, {
    signal,
    skipErrorHandler: true,
    headers: { [PLAYGROUND_FINGERPRINT_HEADER]: getPlaygroundFingerprint() },
  } as Record<string, unknown>)
  return res.data
}

export async function fetchVideoTask(
  taskId: string,
  signal?: AbortSignal
): Promise<VideoTaskFetchResponse> {
  const res = await api.get(
    `${API_ENDPOINTS.VIDEO_GENERATIONS}/${encodeURIComponent(taskId)}`,
    {
      signal,
      skipErrorHandler: true,
      headers: { [PLAYGROUND_FINGERPRINT_HEADER]: getPlaygroundFingerprint() },
    } as Record<string, unknown>
  )
  return res.data
}

export async function submitThreeDTask(
  config: ThreeDWorkbenchConfig,
  prompt: string,
  inputReference: string,
  sourceTaskId: string,
  signal?: AbortSignal
): Promise<ThreeDTaskResponse> {
  const payload: Record<string, unknown> = {
    model: config.model,
    group: config.group,
  }
  if (prompt) {
    payload.prompt = prompt
  }
  if (inputReference) {
    payload.input_reference = inputReference
  }
  if (sourceTaskId) {
    payload.source_task_id = sourceTaskId
  }
  if (config.artStyle) {
    payload.metadata = { art_style: config.artStyle }
  }
  const res = await api.post(API_ENDPOINTS.THREE_D_GENERATIONS, payload, {
    signal,
    skipErrorHandler: true,
    headers: { [PLAYGROUND_FINGERPRINT_HEADER]: getPlaygroundFingerprint() },
  } as Record<string, unknown>)
  return res.data
}

export async function fetchThreeDTask(
  taskId: string,
  signal?: AbortSignal
): Promise<ThreeDTaskResponse> {
  const res = await api.get(
    `${API_ENDPOINTS.THREE_D_GENERATIONS}/${encodeURIComponent(taskId)}`,
    {
      signal,
      skipErrorHandler: true,
      headers: { [PLAYGROUND_FINGERPRINT_HEADER]: getPlaygroundFingerprint() },
    } as Record<string, unknown>
  )
  return res.data
}
