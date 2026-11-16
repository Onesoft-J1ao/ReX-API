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
import type { PlaygroundConfig, ParameterEnabled } from './types'

// Message constants
export const MESSAGE_ROLES = {
  USER: 'user',
  ASSISTANT: 'assistant',
  SYSTEM: 'system',
} as const

export const MESSAGE_STATUS = {
  LOADING: 'loading',
  STREAMING: 'streaming',
  COMPLETE: 'complete',
  ERROR: 'error',
} as const

// API endpoints
export const API_ENDPOINTS = {
  CHAT_COMPLETIONS: '/pg/chat/completions',
  IMAGE_GENERATIONS: '/pg/images/generations',
  IMAGE_EDITS: '/pg/images/edits',
  AUDIO_SPEECH: '/pg/audio/speech',
  AUDIO_TRANSCRIPTIONS: '/pg/audio/transcriptions',
  SPEECH: '/pg/audio/speech',
  SPEECH_TASKS: '/pg/audio/speech/tasks',
  THREE_D: '/pg/3d',
  VIDEOS: '/pg/videos',
  VIDEO_GENERATIONS: '/pg/videos',
  THREE_D_GENERATIONS: '/pg/3d',
  USER_MODELS: '/api/user/models',
  USER_GROUPS: '/api/user/self/groups',
} as const

export const OPENAI_SPEECH_VOICES = [
  'alloy',
  'ash',
  'ballad',
  'coral',
  'echo',
  'fable',
  'nova',
  'onyx',
  'sage',
  'shimmer',
  'verse',
] as const

export const UNREAL_SPEECH_VOICES = [
  { value: 'Autumn', language: 'English' },
  { value: 'Melody', language: 'English' },
  { value: 'Hannah', language: 'English' },
  { value: 'Emily', language: 'English' },
  { value: 'Ivy', language: 'English' },
  { value: 'Kaitlyn', language: 'English' },
  { value: 'Luna', language: 'English' },
  { value: 'Willow', language: 'English' },
  { value: 'Lauren', language: 'English' },
  { value: 'Sierra', language: 'English' },
  { value: 'Noah', language: 'English' },
  { value: 'Jasper', language: 'English' },
  { value: 'Caleb', language: 'English' },
  { value: 'Ronan', language: 'English' },
  { value: 'Ethan', language: 'English' },
  { value: 'Daniel', language: 'English' },
  { value: 'Zane', language: 'English' },
  { value: 'Mei', language: 'Chinese' },
  { value: 'Lian', language: 'Chinese' },
  { value: 'Ting', language: 'Chinese' },
  { value: 'Jing', language: 'Chinese' },
  { value: 'Wei', language: 'Chinese' },
  { value: 'Jian', language: 'Chinese' },
  { value: 'Hao', language: 'Chinese' },
  { value: 'Sheng', language: 'Chinese' },
  { value: 'Lucía', language: 'Spanish' },
  { value: 'Mateo', language: 'Spanish' },
  { value: 'Javier', language: 'Spanish' },
  { value: 'Élodie', language: 'French' },
  { value: 'Ananya', language: 'Hindi' },
  { value: 'Priya', language: 'Hindi' },
  { value: 'Arjun', language: 'Hindi' },
  { value: 'Rohan', language: 'Hindi' },
  { value: 'Giulia', language: 'Italian' },
  { value: 'Luca', language: 'Italian' },
  { value: 'Camila', language: 'Portuguese' },
  { value: 'Thiago', language: 'Portuguese' },
  { value: 'Rafael', language: 'Portuguese' },
] as const

// Default group — uses 'default' as the safe fallback; auto-group is
// only selected when the backend confirms it is available for the user.
export const DEFAULT_GROUP = 'default' as const

// Default configuration
export const DEFAULT_CONFIG: PlaygroundConfig = {
  model: 'gpt-4o',
  group: DEFAULT_GROUP,
  temperature: 0.7,
  top_p: 1,
  max_tokens: 4096,
  frequency_penalty: 0,
  presence_penalty: 0,
  seed: null,
  stream: true,
}

export const DEFAULT_PARAMETER_ENABLED: ParameterEnabled = {
  temperature: true,
  top_p: true,
  max_tokens: false,
  frequency_penalty: true,
  presence_penalty: true,
  seed: false,
}

// Image workbench presets. Sizes and qualities follow the OpenAI image API;
// a provider that rejects a value reports it back through the error message.
export const IMAGE_SIZE_OPTIONS = [
  'auto',
  '1024x1024',
  '1536x1024',
  '1024x1536',
  '1792x1024',
  '1024x1792',
  '512x512',
] as const

export const IMAGE_QUALITY_OPTIONS = ['auto', 'low', 'medium', 'high'] as const

export const IMAGE_N_MAX = 10

export const DEFAULT_IMAGE_CONFIG = {
  model: '',
  group: DEFAULT_GROUP,
  size: 'auto',
  quality: 'auto',
  n: 1,
  inputFidelity: '',
  background: 'auto',
  outputFormat: 'png',
  moderation: 'auto',
  compression: 100,
  timeoutSeconds: 300,
  provider: 'gateway',
  baseUrl: '',
  apiKey: '',
} as const

// Values the OpenAI image endpoints accept. Kept as plain lists: providers
// that do not support one simply echo an error instead of silently dropping it.
export const IMAGE_BACKGROUND_OPTIONS = [
  'auto',
  'transparent',
  'opaque',
] as const
export const IMAGE_OUTPUT_FORMAT_OPTIONS = ['png', 'jpeg', 'webp'] as const
export const IMAGE_MODERATION_OPTIONS = ['auto', 'low'] as const

export const MAX_REFERENCE_IMAGES = 4

// Common OpenAI-compatible voices, offered as suggestions: the field stays free
// text because each provider ships its own set.
export const SPEECH_VOICE_SUGGESTIONS = [
  'alloy',
  'echo',
  'fable',
  'onyx',
  'nova',
  'shimmer',
] as const

// Playback formats the speech endpoint can return. mp3 is the safe default.
export const SPEECH_FORMAT_OPTIONS = [
  'mp3',
  'opus',
  'aac',
  'flac',
  'wav',
  'pcm',
] as const

export const SPEECH_SPEED_MIN = 0.25
export const SPEECH_SPEED_MAX = 4

export const DEFAULT_SPEECH_CONFIG = {
  model: '',
  group: DEFAULT_GROUP,
  voice: 'alloy',
  responseFormat: 'mp3',
  speed: 1,
} as const

// Video task defaults. Agnes 2.5 accepts 4-12 seconds and defaults to 5 when
// the client omits the duration, so the panel opens on the provider default.
export const VIDEO_DURATION_MIN = 4
export const VIDEO_DURATION_MAX = 12
export const VIDEO_DURATION_DEFAULT = 5
export const VIDEO_MAX_REFERENCE_IMAGES = 5

export const DEFAULT_VIDEO_CONFIG = {
  model: '',
  group: DEFAULT_GROUP,
  duration: VIDEO_DURATION_DEFAULT,
} as const

// 3D task defaults. Meshy validates the art style against this list.
export const THREE_D_ART_STYLE_OPTIONS = [
  { value: '', label: 'Provider default' },
  { value: 'realistic', label: 'Realistic' },
  { value: 'cartoon', label: 'Cartoon' },
  { value: 'sculpture', label: 'Sculpture' },
  { value: 'pbr', label: 'PBR' },
] as const

// Meshy models suffixed with -texture re-texture an earlier draft instead of
// creating geometry, and the adaptor requires a source task for them.
export const THREE_D_TEXTURE_MODEL_SUFFIX = '-texture'

export const DEFAULT_THREE_D_CONFIG = {
  model: '',
  group: DEFAULT_GROUP,
  artStyle: '',
} as const

// Both task APIs are asynchronous, so the panels submit, then poll the task
// until it reaches a terminal status.
export const TASK_POLL_INTERVAL_MS = 5000

// Storage keys
export const STORAGE_KEYS = {
  CONFIG: 'playground_config',
  MESSAGES: 'playground_messages',
  PARAMETER_ENABLED: 'playground_parameter_enabled',
} as const

// Error messages
export const ERROR_MESSAGES = {
  API_REQUEST_ERROR: 'Request error occurred',
  NETWORK_ERROR: 'Network connection failed or server not responding',
  PARSE_ERROR: 'Error parsing response data',
  STREAM_START_ERROR: 'Error establishing connection',
  CONNECTION_CLOSED: 'Connection closed',
  INTERRUPTED: 'Generation was interrupted',
} as const

// Message action button styles
export const MESSAGE_ACTION_BUTTON_STYLES = {
  BASE: 'size-7 text-muted-foreground hover:text-foreground',
  DELETE: 'size-7 text-muted-foreground hover:text-destructive',
  ICON: 'size-4',
} as const

// Message action labels
export const MESSAGE_ACTION_LABELS = {
  COPY: 'Copy',
  COPIED: 'Copied!',
  REGENERATE: 'Regenerate',
  SHOW_PREVIEW: 'Show preview',
  SHOW_SOURCE: 'Show source',
  EDIT: 'Edit',
  DELETE: 'Delete',
  NO_CONTENT: 'No content to copy',
  WAIT_GENERATION: 'Please wait for the current generation to complete',
} as const
