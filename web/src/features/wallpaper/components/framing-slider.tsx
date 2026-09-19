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
import { Slider } from '@/components/ui/slider'

export type FramingSliderProps = {
  label: string
  value: number
  min: number
  max: number
  step: number
  format: (value: number) => string
  onChange: (value: number) => void
  /** Rendered on a dark surface, where the muted text colours do not read. */
  tone?: 'surface' | 'overlay'
}

/** One labelled axis of the wallpaper crop, used by the settings and by the
 * on-page framing overlay. */
export function FramingSlider(props: FramingSliderProps) {
  const labelClass =
    props.tone === 'overlay'
      ? 'text-white/70'
      : 'text-muted-foreground'

  return (
    <div className='flex items-center gap-3'>
      <span className={`w-14 shrink-0 text-xs ${labelClass}`}>
        {props.label}
      </span>
      <Slider
        value={props.value}
        min={props.min}
        max={props.max}
        step={props.step}
        aria-label={props.label}
        onValueChange={(next) => {
          const value = Array.isArray(next) ? next[0] : next
          if (typeof value === 'number') props.onChange(value)
        }}
        className='flex-1'
      />
      <span
        className={`w-12 shrink-0 text-right font-mono text-xs tabular-nums ${labelClass}`}
      >
        {props.format(props.value)}
      </span>
    </div>
  )
}
