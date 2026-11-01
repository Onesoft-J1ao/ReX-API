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
import { ChevronDown, RotateCcw } from 'lucide-react'
import {
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import {
  ENDPOINT_TYPES,
  FILTER_ALL,
  QUOTA_TYPES,
  getEndpointTypeLabels,
  getQuotaTypeLabels,
} from '../constants'
import { hasTaskUsageSchema } from '../lib/dynamic-price'
import { parseTags } from '../lib/filters'
import type { PricingModel, PricingVendor } from '../types'

type FilterOption = {
  value: string
  label: string
  count?: number
  /**
   * Shown instead of `count` when set. A group chip has both a ratio and a
   * model count, so `suffix` carries the ratio and `count` becomes the trailing
   * "· 3" in the label.
   */
  suffix?: string
  icon?: ReactNode
}

type FilterSectionProps = {
  title: string
  value: string
  options: FilterOption[]
  // Chips of very different widths wrap raggedly, so a section whose labels
  // vary a lot can ask for an even two-column grid instead.
  layout?: 'wrap' | 'grid'
  onChange: (value: string) => void
}

export interface PricingSidebarProps {
  quotaTypeFilter: string
  endpointTypeFilter: string
  vendorFilter: string
  groupFilter: string
  tagFilter: string
  onQuotaTypeChange: (value: string) => void
  onEndpointTypeChange: (value: string) => void
  onVendorChange: (value: string) => void
  onGroupChange: (value: string) => void
  onTagChange: (value: string) => void
  vendors: PricingVendor[]
  groups: string[]
  groupRatios?: Record<string, number>
  tags: string[]
  models: PricingModel[]
  hasActiveFilters: boolean
  onClearFilters: () => void
  className?: string
}

function countBy(
  models: PricingModel[],
  predicate: (model: PricingModel) => boolean
): number {
  return models.reduce((count, model) => count + (predicate(model) ? 1 : 0), 0)
}

function formatGroupRatio(ratio: number | undefined): string | undefined {
  if (ratio == null) return undefined
  const formatted = Number.isInteger(ratio)
    ? ratio.toString()
    : ratio.toFixed(3).replace(/0+$/, '').replace(/\.$/, '')
  return `x${formatted}`
}

/**
 * Which options need a full row in the two-column grid.
 *
 * A chip carries a label and a ratio badge; when the two do not fit one column
 * the label is ellipsised, and every group sharing a prefix then reads the same
 * ("Claude…"). Measuring the real text at the chip's own font keeps that
 * decision honest across fonts and locales. A spanning chip is two columns plus
 * the gap wide, so it lines up exactly with the pair above it.
 */
function useFullRowOptions(
  container: RefObject<HTMLDivElement | null>,
  options: FilterOption[],
  enabled: boolean
): Set<string> {
  const [fullRow, setFullRow] = useState<Set<string>>(() => new Set())

  useLayoutEffect(() => {
    const element = container.current
    if (!enabled || !element) {
      setFullRow(new Set())
      return
    }

    let cancelled = false
    const measure = () => {
      if (cancelled) return
      const context = document.createElement('canvas').getContext('2d')
      const chip = element.querySelector('button')
      const font = chip ? getComputedStyle(chip).font : ''
      const styles = getComputedStyle(element)
      const gap = Number.parseFloat(styles.columnGap || styles.gap) || 6
      const column = (element.clientWidth - gap) / 2
      if (column <= 0) return
      const textWidth = (text: string) => {
        if (!context) return text.length * 7
        context.font = font
        return context.measureText(text).width
      }
      const next = new Set<string>()
      for (const option of options) {
        // px-2 on both sides, the gap before the badge, and the badge's own
        // horizontal padding (px-1.5).
        const needed =
          textWidth(option.label) +
          (option.suffix ? textWidth(option.suffix) + 6 + 12 : 0) +
          16 +
          6
        if (needed > column) next.add(option.value)
      }
      setFullRow(next)
    }

    measure()
    // The first pass can run before the panel has its final width and before the
    // webfont is applied, and either changes every label's width, so measure
    // again when the box resizes and once the fonts have settled.
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    void document.fonts?.ready.then(measure).catch(() => undefined)
    return () => {
      cancelled = true
      observer.disconnect()
    }
  }, [container, options, enabled])

  return fullRow
}

function FilterChip(props: {
  option: FilterOption
  active: boolean
  fill?: boolean
  wide?: boolean
  onClick: () => void
}) {
  return (
    <Button
      type='button'
      variant={props.active ? 'secondary' : 'outline'}
      size='sm'
      onClick={props.onClick}
      aria-pressed={props.active}
      className={cn(
        'h-auto max-w-full gap-1.5 px-2 py-1 text-xs',
        props.fill && 'w-full justify-between',
        props.fill && props.wide && 'col-span-2'
      )}
      title={props.option.label}
    >
      {props.option.icon && (
        <span className='shrink-0'>{props.option.icon}</span>
      )}
      <span className='truncate'>{props.option.label}</span>
      {props.option.suffix && props.option.count != null && (
        <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
          · {props.option.count}
        </span>
      )}
      {(props.option.suffix || props.option.count != null) && (
        <span
          className={cn(
            'rounded-md px-1.5 py-0.5 text-xs',
            props.active
              ? 'bg-background text-foreground'
              : 'bg-muted text-muted-foreground'
          )}
        >
          {props.option.suffix ?? props.option.count}
        </span>
      )}
    </Button>
  )
}

function FilterSection(props: FilterSectionProps) {
  const gridRef = useRef<HTMLDivElement>(null)
  const fullRow = useFullRowOptions(
    gridRef,
    props.options,
    props.layout === 'grid'
  )

  return (
    <Collapsible
      defaultOpen
      className='border-border/70 border-b pb-3 last:border-b-0'
    >
      <CollapsibleTrigger className='group flex w-full items-center justify-between py-2 text-left'>
        <span className='text-foreground text-sm font-semibold'>
          {props.title}
        </span>
        <ChevronDown className='text-muted-foreground size-4 transition-transform group-data-[panel-open]:rotate-180' />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div
          ref={props.layout === 'grid' ? gridRef : undefined}
          className={cn(
            'gap-1.5',
            props.layout === 'grid'
              ? 'grid grid-cols-2'
              : 'flex flex-wrap'
          )}
        >
          {props.options.map((option) => (
            <FilterChip
              key={option.value}
              option={option}
              active={props.value === option.value}
              fill={props.layout === 'grid'}
              wide={fullRow.has(option.value)}
              onClick={() => props.onChange(option.value)}
            />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

export function PricingSidebar(props: PricingSidebarProps) {
  const { t } = useTranslation()
  const quotaTypeLabels = getQuotaTypeLabels(t)
  const endpointTypeLabels = getEndpointTypeLabels(t)

  const vendorOptions: FilterOption[] = [
    {
      value: FILTER_ALL,
      label: t('All Vendors'),
      count: props.models.length,
    },
    ...props.vendors
      .map((vendor) => ({
        value: vendor.name,
        label: vendor.name,
        count: countBy(
          props.models,
          (model) => model.vendor_name === vendor.name
        ),
        icon: vendor.icon ? getLobeIcon(vendor.icon, 14) : undefined,
      }))
      .filter((vendor) => vendor.count > 0),
  ]

  const groupOptions: FilterOption[] = [
    {
      value: FILTER_ALL,
      label: t('All Groups'),
      count: props.models.length,
    },
    ...props.groups.map((group) => ({
      value: group,
      label: group,
      // As in every other facet here, the badge says how many models the
      // option currently holds. Without it a group that serves nothing looks
      // the same as one that serves half the listing.
      count: countBy(props.models, (model) =>
        (model.enable_groups ?? []).includes(group)
      ),
      suffix: formatGroupRatio(props.groupRatios?.[group]),
    })),
  ]

  const quotaOptions: FilterOption[] = [
    {
      value: QUOTA_TYPES.ALL,
      label: quotaTypeLabels[QUOTA_TYPES.ALL],
      count: props.models.length,
    },
    {
      value: QUOTA_TYPES.TOKEN,
      label: quotaTypeLabels[QUOTA_TYPES.TOKEN],
      count: countBy(
        props.models,
        (model) => model.quota_type === 0 && !hasTaskUsageSchema(model)
      ),
    },
    {
      value: QUOTA_TYPES.REQUEST,
      label: quotaTypeLabels[QUOTA_TYPES.REQUEST],
      count: countBy(
        props.models,
        (model) => model.quota_type === 1 && !hasTaskUsageSchema(model)
      ),
    },
    {
      value: QUOTA_TYPES.TASK,
      label: quotaTypeLabels[QUOTA_TYPES.TASK],
      count: countBy(props.models, (model) => hasTaskUsageSchema(model)),
    },
  ]

  const tagOptions: FilterOption[] = [
    {
      value: FILTER_ALL,
      label: t('All Tags'),
      count: props.models.length,
    },
    ...props.tags.map((tag) => ({
      value: tag,
      label: tag,
      count: countBy(props.models, (model) =>
        parseTags(model.tags)
          .map((item) => item.toLowerCase())
          .includes(tag.toLowerCase())
      ),
    })),
  ]

  const endpointOptions: FilterOption[] = [
    {
      value: ENDPOINT_TYPES.ALL,
      label: endpointTypeLabels[ENDPOINT_TYPES.ALL],
      count: props.models.length,
    },
    ...Object.entries(endpointTypeLabels)
      .filter(([value]) => value !== ENDPOINT_TYPES.ALL)
      .map(([value, label]) => ({
        value,
        label,
        count: countBy(
          props.models,
          (model) => model.supported_endpoint_types?.includes(value) ?? false
        ),
      })),
  ]

  return (
    <aside className={cn('bg-card/80 rounded-xl border p-3 backdrop-blur-sm', props.className)}>
      <div className='mb-2.5 flex items-center justify-between gap-2'>
        <div>
          <h2 className='text-foreground text-sm font-bold'>{t('Filter')}</h2>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('Refine models by provider, group, type, and tags.')}
          </p>
        </div>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          onClick={props.onClearFilters}
          disabled={!props.hasActiveFilters}
          className='h-7 gap-1.5 px-2 text-xs'
        >
          <RotateCcw className='size-3.5' />
          {t('Reset')}
        </Button>
      </div>

      {props.hasActiveFilters && (
        <Badge variant='secondary' className='mb-3'>
          {t('Filters active')}
        </Badge>
      )}

      <div className='space-y-1'>
        <FilterSection
          title={t('Groups')}
          value={props.groupFilter}
          options={groupOptions}
          layout='grid'
          onChange={props.onGroupChange}
        />
        <FilterSection
          title={t('All Vendors')}
          value={props.vendorFilter}
          options={vendorOptions}
          onChange={props.onVendorChange}
        />
        <FilterSection
          title={t('Model Tags')}
          value={props.tagFilter}
          options={tagOptions}
          onChange={props.onTagChange}
        />
        <FilterSection
          title={t('Pricing Type')}
          value={props.quotaTypeFilter}
          options={quotaOptions}
          onChange={props.onQuotaTypeChange}
        />
        <FilterSection
          title={t('Endpoint Type')}
          value={props.endpointTypeFilter}
          options={endpointOptions}
          onChange={props.onEndpointTypeChange}
        />
      </div>
    </aside>
  )
}
