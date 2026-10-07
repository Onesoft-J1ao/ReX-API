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
/**
 * LobeHub Icon Loader
 * Dynamically load and render icons from @lobehub/icons
 *
 * Supports:
 * - Basic: "OpenAI", "OpenAI.Color"
 * - Chained properties: "OpenAI.Avatar.type={'platform'}"
 * - Size parameter: getLobeIcon("OpenAI", 20)
 */
import type React from 'react'

import bigcodeLogo from '@/assets/brand-icons/bigcode.svg'
import deepgramLogo from '@/assets/brand-icons/deepgram.svg'
import mineruLogo from '@/assets/brand-icons/mineru.svg'
import openbmbLogo from '@/assets/brand-icons/openbmb.svg'
import opendatalabLogo from '@/assets/brand-icons/opendatalab.svg'
import sglangLogo from '@/assets/brand-icons/sglang.svg'
import teleaiLogo from '@/assets/brand-icons/teleai.svg'
import typesafeLogo from '@/assets/brand-icons/typesafe.svg'
import unrealspeechLogo from '@/assets/brand-icons/unrealspeech.svg'
import writerLogo from '@/assets/brand-icons/writer.svg'
import zyphraLogo from '@/assets/brand-icons/zyphra.svg'
import { IconSub2api } from '@/assets/custom/icon-sub2api'
import { IconWan } from '@/assets/custom/icon-wan'
import { BrandLogo } from './brand-logo'
import { IconFallbackBadge } from './icon-fallback-badge'
import * as LobeIconRegistry from './lobe-icon-registry'
import type { LobeIconComponent } from './lobe-icon-registry'

// The registry is ~5 MB of SVG path data. Importing it statically (rather than
// behind a dynamic import) keeps the lookup synchronous — a provider badge
// always renders its glyph on the first paint, with no placeholder flash — and
// the vendor-lobe-icons cache group in rsbuild.config.ts keeps it in its own
// long-lived chunk, so a deploy does not force a re-download. A dynamic import
// was tried and reverted: the package declares sideEffects: false, so the
// bundler folded the whole registry back into the entry chunk anyway while
// making the render path asynchronous.
//

const CUSTOM_ICONS: Record<string, React.ComponentType<{ size?: number }>> = {
  SGLang: (props) => (
    <img
      src={sglangLogo}
      alt=''
      aria-hidden='true'
      width={props.size ?? 20}
      height={props.size ?? 20}
      className='object-contain'
    />
  ),
  Sub2API: IconSub2api,
  Wan: IconWan,
  Writer: (props) => <BrandLogo src={writerLogo} size={props.size} mono />,
  Deepgram: (props) => <BrandLogo src={deepgramLogo} size={props.size} mono />,
  BigCode: (props) => <BrandLogo src={bigcodeLogo} size={props.size} />,
  MinerU: (props) => <BrandLogo src={mineruLogo} size={props.size} mono />,
  OpenDataLab: (props) => (
    <BrandLogo src={opendatalabLogo} size={props.size} />
  ),
  OpenBMB: (props) => <BrandLogo src={openbmbLogo} size={props.size} />,
  TeleAI: (props) => <BrandLogo src={teleaiLogo} size={props.size} />,
  TypeSafe: (props) => <BrandLogo src={typesafeLogo} size={props.size} mono />,
  UnrealSpeech: (props) => (
    <BrandLogo src={unrealspeechLogo} size={props.size} mono />
  ),
  Zyphra: (props) => <BrandLogo src={zyphraLogo} size={props.size} mono />,
}

/**
 * Parse a property value from string to appropriate type
 * @param raw - Raw string value
 * @returns Parsed value (boolean, number, or string)
 */
function parseValue(raw: string | undefined | null): string | number | boolean {
  if (raw == null) return true

  let v = String(raw).trim()

  // Remove curly braces
  if (v.startsWith('{') && v.endsWith('}')) {
    v = v.slice(1, -1).trim()
  }

  // Remove quotes
  if (
    (v.startsWith('"') && v.endsWith('"')) ||
    (v.startsWith("'") && v.endsWith("'"))
  ) {
    return v.slice(1, -1)
  }

  // Boolean
  if (v === 'true') return true
  if (v === 'false') return false

  // Number
  if (/^-?\d+(?:\.\d+)?$/.test(v)) return Number(v)

  // Return as string
  return v
}

/**
 * Get LobeHub icon component by name
 * @param iconName - Icon name/description (e.g., "OpenAI", "OpenAI.Color", "Claude.Avatar")
 * @param size - Icon size (default: 20)
 * @returns Icon component or fallback
 *
 * @example
 * getLobeIcon("OpenAI", 24)
 * getLobeIcon("OpenAI.Color", 20)
 * getLobeIcon("Claude.Avatar.type={'platform'}", 32)
 */
export function getLobeIcon(
  iconName: string | undefined | null,
  size: number = 20
): React.ReactNode {
  if (!iconName || typeof iconName !== 'string') {
    return (
      <IconFallbackBadge label='?' size={size} />
    )
  }

  const trimmedName = iconName.trim()
  if (!trimmedName) {
    return (
      <IconFallbackBadge label='?' size={size} />
    )
  }

  // Parse component path and chained properties
  const segments = trimmedName.split('.')
  const baseKey = segments[0]
  const CustomIcon = CUSTOM_ICONS[baseKey]
  if (CustomIcon) {
    return <CustomIcon size={size} />
  }
  const BaseIcon = LobeIconRegistry.resolveLobeIcon(baseKey)

  let IconComponent: React.ComponentType<Record<string, unknown>> | undefined
  let propStartIndex: number

  if (BaseIcon && segments.length > 1 && BaseIcon[segments[1]]) {
    IconComponent = BaseIcon[segments[1]] as React.ComponentType<
      Record<string, unknown>
    >
    propStartIndex = 2
  } else {
    IconComponent = LobeIconRegistry.lobeIconRegistry[baseKey] as
      | LobeIconComponent
      | undefined
    propStartIndex = segments.length > 1 && /^[A-Z]/.test(segments[1]) ? 2 : 1
  }

  // Fallback if icon not found
  if (
    !IconComponent ||
    (typeof IconComponent !== 'function' && typeof IconComponent !== 'object')
  ) {
    return <IconFallbackBadge
      label={trimmedName.charAt(0).toUpperCase()}
      size={size}
    />
  }

  // Parse chained properties (e.g., "type={'platform'}", "shape='square'")
  const props: Record<string, string | number | boolean> = {}

  for (let i = propStartIndex; i < segments.length; i++) {
    const seg = segments[i]
    if (!seg) continue

    const eqIdx = seg.indexOf('=')
    if (eqIdx === -1) {
      props[seg.trim()] = true
      continue
    }

    const key = seg.slice(0, eqIdx).trim()
    const valRaw = seg.slice(eqIdx + 1).trim()
    props[key] = parseValue(valRaw)
  }

  // Set size if not explicitly specified in the string
  if (props.size == null && size != null) {
    props.size = size
  }

  return <IconComponent {...props} />
}

// The selector uses the same installed icon registry as the renderer.
export function getLobeIconNames(): string[] {
  const names = LobeIconRegistry.lobeIconNames()
  return [...new Set([...names, ...Object.keys(CUSTOM_ICONS)])].sort()
}
