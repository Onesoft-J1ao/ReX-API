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
/*
 * Curated LobeHub icon registry.
 *
 * `@lobehub/icons` ships ~325 icons. Importing the namespace pulled every SVG
 * path into one 4.9 MB chunk (0.87 MB gzipped) that had to arrive before first
 * paint. Only the providers this deployment actually renders are imported
 * below; unknown keys fall back to IconFallbackBadge.
 *
 * Add a provider by importing its module and adding it to ICONS - keep the
 * list alphabetical so merges stay trivial.
 */

import I0 from '@lobehub/icons/es/AgnesAI'
import I1 from '@lobehub/icons/es/Ai21'
import I2 from '@lobehub/icons/es/Ai360'
import I75 from '@lobehub/icons/es/AlibabaCloud'
import I76 from '@lobehub/icons/es/AntGroup'
import I3 from '@lobehub/icons/es/Anthropic'
import I4 from '@lobehub/icons/es/AssemblyAI'
import I5 from '@lobehub/icons/es/Aws'
import I6 from '@lobehub/icons/es/Azure'
import I7 from '@lobehub/icons/es/BAAI'
import I8 from '@lobehub/icons/es/Baichuan'
import I9 from '@lobehub/icons/es/Bedrock'
import I10 from '@lobehub/icons/es/Bfl'
import I77 from '@lobehub/icons/es/BilibiliIndex'
import I11 from '@lobehub/icons/es/ChatGLM'
import I12 from '@lobehub/icons/es/CherryStudio'
import I13 from '@lobehub/icons/es/Claude'
import I14 from '@lobehub/icons/es/Cloudflare'
import I15 from '@lobehub/icons/es/Cohere'
import I16 from '@lobehub/icons/es/ComfyUI'
import I78 from '@lobehub/icons/es/Dbrx'
import I17 from '@lobehub/icons/es/DeepSeek'
import I18 from '@lobehub/icons/es/Dify'
import I19 from '@lobehub/icons/es/Doubao'
import I20 from '@lobehub/icons/es/ElevenLabs'
import I21 from '@lobehub/icons/es/Figma'
import I22 from '@lobehub/icons/es/Fireworks'
import I23 from '@lobehub/icons/es/FishAudio'
import I24 from '@lobehub/icons/es/Flux'
import I25 from '@lobehub/icons/es/Gemini'
import I26 from '@lobehub/icons/es/Github'
import I27 from '@lobehub/icons/es/Google'
import I28 from '@lobehub/icons/es/Grok'
import I29 from '@lobehub/icons/es/Groq'
import I30 from '@lobehub/icons/es/Hedra'
import I31 from '@lobehub/icons/es/HuggingFace'
import I32 from '@lobehub/icons/es/Hunyuan'
import I79 from '@lobehub/icons/es/IBM'
import I33 from '@lobehub/icons/es/Ideogram'
import I34 from '@lobehub/icons/es/InternLM'
import I35 from '@lobehub/icons/es/Jimeng'
import I36 from '@lobehub/icons/es/Jina'
import I37 from '@lobehub/icons/es/Kimi'
import I38 from '@lobehub/icons/es/Kling'
import I39 from '@lobehub/icons/es/Kolors'
import I40 from '@lobehub/icons/es/LangChain'
import I80 from '@lobehub/icons/es/LongCat'
import I41 from '@lobehub/icons/es/Luma'
import I42 from '@lobehub/icons/es/Meshy'
import I43 from '@lobehub/icons/es/Meta'
import I44 from '@lobehub/icons/es/Microsoft'
import I45 from '@lobehub/icons/es/Midjourney'
import I46 from '@lobehub/icons/es/Minimax'
import I47 from '@lobehub/icons/es/Mistral'
import I48 from '@lobehub/icons/es/Moonshot'
import I49 from '@lobehub/icons/es/NewAPI'
import I50 from '@lobehub/icons/es/Notion'
import I51 from '@lobehub/icons/es/NousResearch'
import I52 from '@lobehub/icons/es/Nvidia'
import I53 from '@lobehub/icons/es/Ollama'
import I54 from '@lobehub/icons/es/OpenAI'
import I81 from '@lobehub/icons/es/OpenCode'
import I55 from '@lobehub/icons/es/OpenRouter'
import I56 from '@lobehub/icons/es/Perplexity'
import I57 from '@lobehub/icons/es/PixVerse'
import I58 from '@lobehub/icons/es/Qwen'
import I59 from '@lobehub/icons/es/Recraft'
import I60 from '@lobehub/icons/es/Replicate'
import I61 from '@lobehub/icons/es/Runway'
import I62 from '@lobehub/icons/es/Sora'
import I63 from '@lobehub/icons/es/Spark'
import I64 from '@lobehub/icons/es/Stability'
import I65 from '@lobehub/icons/es/Stepfun'
import I66 from '@lobehub/icons/es/Suno'
import I67 from '@lobehub/icons/es/Together'
import I68 from '@lobehub/icons/es/Vercel'
import I69 from '@lobehub/icons/es/Vidu'
import I70 from '@lobehub/icons/es/Vllm'
import I71 from '@lobehub/icons/es/Wenxin'
import I72 from '@lobehub/icons/es/XiaomiMiMo'
import I73 from '@lobehub/icons/es/Yi'
import I82 from '@lobehub/icons/es/ZAI'
import I74 from '@lobehub/icons/es/Zhipu'
import type React from 'react'

const ICONS: Record<string, unknown> = {
  AgnesAI: I0,
  Ai21: I1,
  Ai360: I2,
  Anthropic: I3,
  AssemblyAI: I4,
  Aws: I5,
  Azure: I6,
  BAAI: I7,
  Baichuan: I8,
  Bedrock: I9,
  Bfl: I10,
  ChatGLM: I11,
  CherryStudio: I12,
  Claude: I13,
  Cloudflare: I14,
  Cohere: I15,
  ComfyUI: I16,
  DeepSeek: I17,
  Dify: I18,
  Doubao: I19,
  ElevenLabs: I20,
  Figma: I21,
  Fireworks: I22,
  FishAudio: I23,
  Flux: I24,
  Gemini: I25,
  Github: I26,
  Google: I27,
  Grok: I28,
  Groq: I29,
  Hedra: I30,
  HuggingFace: I31,
  Hunyuan: I32,
  Ideogram: I33,
  InternLM: I34,
  Jimeng: I35,
  Jina: I36,
  Kimi: I37,
  Kling: I38,
  Kolors: I39,
  LangChain: I40,
  Luma: I41,
  Meshy: I42,
  Meta: I43,
  Microsoft: I44,
  Midjourney: I45,
  Minimax: I46,
  Mistral: I47,
  Moonshot: I48,
  NewAPI: I49,
  Notion: I50,
  NousResearch: I51,
  Nvidia: I52,
  Ollama: I53,
  OpenAI: I54,
  OpenRouter: I55,
  Perplexity: I56,
  PixVerse: I57,
  Qwen: I58,
  Recraft: I59,
  Replicate: I60,
  Runway: I61,
  Sora: I62,
  Spark: I63,
  Stability: I64,
  Stepfun: I65,
  Suno: I66,
  Together: I67,
  Vercel: I68,
  Vidu: I69,
  Vllm: I70,
  Wenxin: I71,
  XiaomiMiMo: I72,
  Yi: I73,
  Zhipu: I74,
  AlibabaCloud: I75,
  AntGroup: I76,
  BilibiliIndex: I77,
  Dbrx: I78,
  IBM: I79,
  LongCat: I80,
  OpenCode: I81,
  ZAI: I82,
}

export const lobeIconRegistry = ICONS

const ICON_NAMES: string[] = Object.keys(ICONS).flatMap((name) => {
  const entry = ICONS[name] as Record<string, unknown> | undefined
  return entry && entry.Color ? [name, `${name}.Color`] : [name]
})

export function lobeIconNames(): string[] {
  return ICON_NAMES
}

export function resolveLobeIcon(
  baseKey: string
): Record<string, unknown> | undefined {
  return lobeIconRegistry[baseKey] as Record<string, unknown> | undefined
}

export type LobeIconComponent = React.ComponentType<Record<string, unknown>>
