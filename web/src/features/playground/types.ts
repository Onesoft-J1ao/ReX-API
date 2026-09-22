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
// Message types
export type MessageRole = 'user' | 'assistant' | 'system'

export type MessageStatus = 'loading' | 'streaming' | 'complete' | 'error'

export type PlaygroundMessageLayoutMode = 'alternating' | 'left'

export interface MessageVersion {
  id: string
  content: string
}

export interface Message {
  key: string
  from: MessageRole
  versions: MessageVersion[]
  createdAt?: number
  startedAt?: number
  completedAt?: number
  durationMs?: number
  sources?: { href: string; title: string }[]
  reasoning?: {
    content: string
    duration: number
    startedAt?: number
    completedAt?: number
    durationMs?: number
  }
  isReasoningStreaming?: boolean
  isReasoningComplete?: boolean
  isContentComplete?: boolean
  status?: MessageStatus
  errorCode?: string | null
}

// API payload types
export interface ChatCompletionMessage {
  role: MessageRole
  content: string | ContentPart[]
}

export interface ContentPart {
  type: 'text' | 'image_url'
  text?: string
  image_url?: {
    url: string
  }
}

export interface ChatCompletionRequest {
  model: string
  group?: string
  messages: ChatCompletionMessage[]
  stream: boolean
  temperature?: number
  top_p?: number
  max_tokens?: number
  frequency_penalty?: number
  presence_penalty?: number
  seed?: number
}

export interface ChatCompletionChunk {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    delta: {
      role?: MessageRole
      content?: string
      reasoning_content?: string
    }
    finish_reason: string | null
  }>
}

export interface ChatCompletionResponse {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    message: {
      role: MessageRole
      content: string
      reasoning_content?: string
    }
    finish_reason: string
  }>
  usage?: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

// Configuration types
export interface PlaygroundConfig {
  model: string
  group: string
  temperature: number
  top_p: number
  max_tokens: number
  frequency_penalty: number
  presence_penalty: number
  seed: number | null
  stream: boolean
}

export interface ParameterEnabled {
  temperature: boolean
  top_p: boolean
  max_tokens: boolean
  frequency_penalty: boolean
  presence_penalty: boolean
  seed: boolean
}

// Model and group options
export interface ModelOption {
  label: string
  value: string
}

export interface GroupOption {
  label: string
  value: string
  ratio: number
  desc?: string
}

// Image workbench types
export type ImageWorkbenchMode = 'text-to-image' | 'image-to-image'

export interface ImageWorkbenchConfig {
  model: string
  group: string
  size: string
  quality: string
  n: number
  inputFidelity: string
}

export interface GeneratedImage {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

export interface SpeechWorkbenchConfig {
  model: string
  group: string
  voice: string
  responseFormat: string
  speed: number
}

export interface TranscriptionResponse {
  text?: string
}

export interface ImageGenerationResponse {
  created?: number
  data?: GeneratedImage[]
}

// Video workbench types. The mode values are the provider modes the Agnes
// adaptor validates, so the panel sends the selection without translating it.
export type VideoWorkbenchMode = 'text' | 'reference' | 'keyframe'

export interface VideoWorkbenchConfig {
  model: string
  group: string
  duration: number
}

export interface VideoTaskInput {
  mode: VideoWorkbenchMode
  prompt: string
  referenceImages: string[]
  firstFrame: string
  lastFrame: string
}

export interface VideoTask {
  taskId: string
  status: string
  progress: string
  resultUrl: string
  failReason: string
}

export interface VideoTaskSubmitResponse {
  id?: string
  task_id?: string
}

// The task endpoints answer with the gateway's generic task record: the video
// relay has no OpenAI-shaped converter, so the fetch response is wrapped.
export interface VideoTaskFetchResponse {
  code?: string
  data?: {
    task_id?: string
    status?: string
    progress?: string
    result_url?: string
    fail_reason?: string
  }
}

// 3D workbench types. Meshy answers submit and fetch with the same object.
export interface ThreeDWorkbenchConfig {
  model: string
  group: string
  artStyle: string
}

export interface ThreeDTask {
  taskId: string
  status: string
  progress: number
  format: string
  url: string
  failReason: string
}

export interface ThreeDTaskResponse {
  id?: string
  object?: string
  model?: string
  status?: string
  progress?: number
  created_at?: number
  data?: {
    format?: string
    url?: string
  }
  error?: {
    code?: string
    message?: string
  }
}
