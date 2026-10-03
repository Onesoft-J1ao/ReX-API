/* Copyright (C) 2023-2026 QuantumNous */
import axios from 'axios'

import type { GroupOption, ModelOption } from '../../types'

export function filterGenerationGroups(
  groups: GroupOption[],
  groupModels: Record<string, string[]>,
  models: ModelOption[]
): GroupOption[] {
  const allowedModels = new Set(models.map((model) => model.value))
  return groups.filter((group) => {
    if (group.value === 'auto') return false
    const availableModels = groupModels[group.value] ?? []
    return availableModels.some((model) => allowedModels.has(model))
  })
}

export function resolveGenerationGroup(
  groups: GroupOption[],
  groupModels: Record<string, string[]>,
  models: ModelOption[],
  selectedGroup: string
): string {
  const eligibleGroups = filterGenerationGroups(groups, groupModels, models)
  if (eligibleGroups.some((group) => group.value === selectedGroup)) {
    return selectedGroup
  }
  return eligibleGroups[0]?.value ?? ''
}

export function filterGenerationModels(
  models: ModelOption[],
  allowedModels: string[]
): ModelOption[] {
  if (allowedModels.length === 0) return models
  const allowed = new Set(allowedModels)
  return models.filter((model) => allowed.has(model.value))
}

export function filterGenerationModelsForGroup(
  models: ModelOption[],
  groupModels: Record<string, string[]>,
  selectedGroup: string
): ModelOption[] {
  if (!selectedGroup) return []
  const availableModels = new Set(groupModels[selectedGroup] ?? [])
  return models.filter((model) => availableModels.has(model.value))
}

export function imageResponseSource(image: {
  url?: string
  b64_json?: string
}): string {
  if (image.url) return image.url
  return image.b64_json ? `data:image/png;base64,${image.b64_json}` : ''
}

export function normalizeImageAspectRatio(value: string): string | null {
  const match = value.match(/^\s*(\d+)\s*:\s*(\d+)\s*$/)
  if (!match) return null

  let width = Number(match[1])
  let height = Number(match[2])
  if (
    !Number.isSafeInteger(width) ||
    !Number.isSafeInteger(height) ||
    width <= 0 ||
    height <= 0
  ) {
    return null
  }

  let left = width
  let right = height
  while (right !== 0) {
    const remainder = left % right
    left = right
    right = remainder
  }
  width /= left
  height /= left
  return `${width}:${height}`
}

export function imageSizeFromResolution(
  resolution: number,
  aspectRatio: string
): string | null {
  const normalizedRatio = normalizeImageAspectRatio(aspectRatio)
  if (
    !normalizedRatio ||
    !Number.isSafeInteger(resolution) ||
    resolution <= 0
  ) {
    return null
  }

  const [widthUnit, heightUnit] = normalizedRatio.split(':').map(Number)
  const maxUnit = Math.max(widthUnit, heightUnit)
  const rawScale = Math.floor(resolution / maxUnit)
  const alignedScale = Math.floor(rawScale / 8) * 8
  const scale = alignedScale || rawScale

  if (scale > 0) {
    return `${widthUnit * scale}x${heightUnit * scale}`
  }

  if (widthUnit > heightUnit) {
    return `${resolution}x${Math.max(1, Math.round((resolution * heightUnit) / widthUnit))}`
  }
  return `${Math.max(1, Math.round((resolution * widthUnit) / heightUnit))}x${resolution}`
}

export async function getGenerationErrorMessage(
  error: unknown,
  fallback: string
): Promise<string> {
  if (!axios.isAxiosError(error)) {
    return error instanceof Error ? error.message : fallback
  }
  const data = error.response?.data
  if (data instanceof Blob) {
    try {
      const parsed = JSON.parse(await data.text()) as {
        error?: { message?: string }
        message?: string
      }
      return parsed.error?.message ?? parsed.message ?? fallback
    } catch {
      return fallback
    }
  }
  if (data && typeof data === 'object') {
    const body = data as { error?: { message?: string }; message?: string }
    return body.error?.message ?? body.message ?? fallback
  }
  return error.message || fallback
}

export function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.addEventListener(
      'load',
      () => resolve(String(reader.result ?? '')),
      {
        once: true,
      }
    )
    reader.addEventListener(
      'error',
      () => reject(reader.error ?? new Error('File read failed')),
      { once: true }
    )
    reader.readAsDataURL(file)
  })
}

/** What the pricing payload publishes about one model's abilities. */
export type GenerationModelMeta = {
  endpoints: string[]
  outputs: string[]
}

export type GenerationKind = 'image' | 'video'

/**
 * The endpoint or output modality that marks a model as serving a kind.
 *
 * Speech and 3D are absent on purpose: the payload publishes no audio marker,
 * and neither picker has a marker to test against, so guessing one would hide
 * working models.
 */
const GENERATION_MARKERS: Record<
  GenerationKind,
  { endpoint?: string; output: string }
> = {
  image: { endpoint: 'image-generation', output: 'image' },
  video: { output: 'video' },
}

/**
 * Keep only the models that can serve a generation kind.
 *
 * The pickers drew from every model the account can reach, so a chat model was
 * selectable in the image and video tabs even though neither can generate with
 * it.
 *
 * A model the payload does not describe at all is kept: "no metadata" is not
 * evidence of being text-only. For a model it does describe, a declared marker
 * keeps it and a declared output of another kind drops it. The remaining case is
 * a model with outputs unstated — common, because most channels publish none.
 * There the endpoints decide: a model that declares endpoints, none of them this
 * kind's, is dropped. That is the case chat models were arriving through, since
 * they declare the chat endpoints and nothing else. A kind with no endpoint
 * marker (video) has nothing left to test and keeps the model.
 */
export function filterModelsForGeneration(
  models: ModelOption[],
  meta: Map<string, GenerationModelMeta>,
  kind: GenerationKind
): ModelOption[] {
  const marker = GENERATION_MARKERS[kind]
  return models.filter((model) => {
    const info = meta.get(model.value)
    if (!info) return true
    if (marker.endpoint && info.endpoints.includes(marker.endpoint)) return true
    if (info.outputs.includes(marker.output)) return true
    if (info.outputs.length > 0) return false
    if (!marker.endpoint) return true
    return info.endpoints.length === 0
  })
}

/**
 * Resolve the model list for a generation tab.
 *
 * An operator-configured list wins: it is an explicit statement of what the
 * instance means for that tab. When none is configured the list is narrowed by
 * what the model can actually do, which is where the text models were leaking
 * in. A model the payload does not describe survives that filter, so an
 * instance whose models carry no metadata keeps its picker unchanged.
 */
export function selectGenerationModels(
  models: ModelOption[],
  published: string[] | null | undefined,
  meta: Map<string, GenerationModelMeta>,
  kind: GenerationKind
): ModelOption[] {
  if (published && published.length > 0) {
    return filterGenerationModels(models, published)
  }
  return filterModelsForGeneration(models, meta, kind)
}
