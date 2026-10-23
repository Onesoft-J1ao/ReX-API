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
import { ChevronRight } from 'lucide-react'
import { memo, useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardFooter, CardHeader } from '@/components/ui/card'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { DEFAULT_TOKEN_UNIT } from '../constants'
import { useBillingTime } from '../hooks/use-billing-time'
import { formatBillingCondition } from '../lib/billing-expression/condition-display'
import {
  formatTaskUsageUnitPrice,
  getCardExamplePrice,
  getDynamicDisplayGroupRatio,
  getDynamicPriceEntries,
  getDynamicPriceUnitLabelKey,
  getDynamicPricingSummary,
  getDynamicPricingTiers,
  isDynamicPricingFree,
  isUnconfiguredTaskUsageModel,
} from '../lib/dynamic-price'
import { parseTags } from '../lib/filters'
import { getGroupPricingForModel } from '../lib/group-pricing'
import { getDisplayGroupRatio } from '../lib/model-helpers'
import {
  formatPrice,
  formatRequestPrice,
  resolveGroupTokenPrices,
  type GroupPricingOverrides,
} from '../lib/price'
import { taskPriceLabel, taskUsageUnitLabel } from '../lib/task-price-display'
import type { PricingModel, PriceType, TokenUnit } from '../types'
import { ModelBillingModeBadge } from './model-billing-mode-badge'
import { ModelPerfBadge, type ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardProps {
  model: PricingModel
  onClick: () => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  /** Per-group pricing source overrides; absent when none are configured. */
  groupBillingSource?: Record<string, Record<string, string>>
  /** Per-group per-model token overrides; absent when none are configured. */
  groupPricingOverrides?: GroupPricingOverrides
  perf?: ModelPerfBadgeData
}

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t, i18n } = useTranslation()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M'

  // A group may bill this model per request even though the model itself is
  // token-priced. The token prices below would then describe a price nobody is
  // charged, so the card asks which source actually applies in the group the
  // viewer is looking at.
  const groupPricing = getGroupPricingForModel(
    props.model,
    props.selectedGroup ?? '',
    props.groupBillingSource,
    1
  )
  const effectiveSource = groupPricing.source
  // A group may pin this model to plain per-token billing with its own ratio.
  // The expression tiers then describe a price this group never pays, so the
  // card must show the pinned prices instead. Only an actual override counts:
  // without one the group just follows the model's global mode.
  const pinnedTokenPrices = useMemo(
    () =>
      props.selectedGroup
        ? resolveGroupTokenPrices(
            props.model,
            props.selectedGroup,
            props.groupBillingSource,
            getDisplayGroupRatio(props.model, props.selectedGroup),
            props.groupPricingOverrides ?? {}
          )
        : null,
    [
      props.model,
      props.selectedGroup,
      props.groupBillingSource,
      props.groupPricingOverrides,
    ]
  )
  // The group pinned a token-priced model to per-request billing. The mode
  // badge still reads from the model, so the card says which one wins here.
  const pricedPerRequest =
    effectiveSource === 'price' && (props.model.model_price ?? 0) > 0
  const pricedPerToken =
    effectiveSource !== 'price' && props.model.quota_type !== 1
  const perRequestOverride = pricedPerRequest && props.model.quota_type !== 1
  const tags = parseTags(props.model.tags)
  const groups = props.model.enable_groups || []
  const endpoints = props.model.supported_endpoint_types || []
  const modelIconKey = props.model.icon || props.model.vendor_icon
  const modelIcon = modelIconKey ? getLobeIcon(modelIconKey, 28) : null
  const initial = props.model.model_name?.charAt(0).toUpperCase() || '?'
  const isUnconfiguredTaskUsage = isUnconfiguredTaskUsageModel(props.model)
  const billingTime = useBillingTime(props.model.billing_expr)
  const currency = useSystemConfigStore((state) => state.config.currency)
  const dynamicPriceOptions = useMemo(
    () => ({
      now: billingTime === undefined ? undefined : new Date(billingTime),
      tokenUnit,
      showRechargePrice,
      priceRate,
      usdExchangeRate,
      groupRatioMultiplier: getDynamicDisplayGroupRatio(
        props.model,
        props.selectedGroup
      ),
    }),
    [
      props.model,
      props.selectedGroup,
      billingTime,
      tokenUnit,
      showRechargePrice,
      priceRate,
      usdExchangeRate,
    ]
  )
  const dynamicSummary = useMemo(
    () => getDynamicPricingSummary(props.model, dynamicPriceOptions),
    // Currency is read indirectly by the price formatter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [props.model, dynamicPriceOptions, currency]
  )
  const cardExamplePrice = useMemo(
    () => getCardExamplePrice(props.model, dynamicPriceOptions),
    // Currency is read indirectly by the price formatter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [props.model, dynamicPriceOptions, currency]
  )
  // A tiered expression keyed on the clock has one price per period, and the
  // summary only carries the period that is current. Listing every period is
  // what makes a peak / off-peak model legible at a glance instead of showing
  // one number the reader cannot place.
  const timeSegments = useMemo(() => {
    if (!dynamicSummary?.isTimePricing) return []
    const currentLabel = dynamicSummary.tier?.label
    return getDynamicPricingTiers(props.model).map((tier) => {
      const entries = getDynamicPriceEntries(tier, dynamicPriceOptions)
      const prices = ['inputPrice', 'outputPrice']
        .map((field) => entries.find((entry) => entry.field === field))
        .filter((entry) => entry != null)
        .map((entry) => entry.formatted)
      // conditionText only exists on the expression tier, not the task tier.
      const conditionText =
        'conditionText' in tier ? (tier.conditionText as string) : undefined
      const condition = conditionText
        ? (formatBillingCondition(conditionText, t, i18n.language) ??
          conditionText)
        : null
      return {
        key: `${tier.label}-${conditionText ?? ''}`,
        label: condition ?? tier.label ?? t('Default'),
        isCurrent: tier.label === currentLabel,
        prices: prices.join(' · '),
      }
    })
  }, [props.model, dynamicSummary, dynamicPriceOptions, t, i18n.language])

  const dynamicIsFree =
    dynamicSummary != null &&
    isDynamicPricingFree(
      dynamicSummary,
      getDynamicDisplayGroupRatio(props.model, props.selectedGroup)
    )
  let priceSummary: ReactNode
  if (pinnedTokenPrices) {
    const pinnedPrices: { type: PriceType; label: string; value: number }[] = [
      { type: 'input', label: t('Input'), value: pinnedTokenPrices.input },
      { type: 'output', label: t('Output'), value: pinnedTokenPrices.output },
      ...(pinnedTokenPrices.cache == null
        ? []
        : [
            {
              type: 'cache' as const,
              label: t('Cache Read'),
              value: pinnedTokenPrices.cache,
            },
          ]),
    ]
    priceSummary = pinnedPrices.map((price) => (
      <div key={price.type} className='flex min-w-0 flex-col gap-1'>
        <span className='text-muted-foreground truncate text-xs whitespace-nowrap'>
          {price.label}
        </span>
        <span className='font-mono text-[14px] font-semibold whitespace-nowrap tabular-nums'>
          {formatTaskUsageUnitPrice(price.value, {
            tokenUnit,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            groupRatioMultiplier: 1,
          })}
          <span className='text-muted-foreground text-xs font-normal'>
            {' '}
            / {tokenUnitLabel}
          </span>
        </span>
      </div>
    ))
  } else if (dynamicSummary && dynamicIsFree) {
    priceSummary = (
      <span className='col-span-full font-mono text-[14px] font-semibold tabular-nums'>
        {t('Free')}
      </span>
    )
  } else if (dynamicSummary) {
    if (dynamicSummary.isSpecialExpression) {
      priceSummary = (
        <div className='col-span-full min-w-0'>
          <span className='text-warning'>
            {t('Special billing expression')}
          </span>
          <code className='text-muted-foreground mt-1 line-clamp-2 block font-mono text-xs break-all'>
            {dynamicSummary.rawExpression}
          </code>
        </div>
      )
    } else if (dynamicSummary.primaryEntries.length > 0) {
      priceSummary = (
        <>
          {[
            ...dynamicSummary.primaryEntries.slice(
              0,
              dynamicSummary.providerCount ? 2 : undefined
            ),
            ...dynamicSummary.secondaryEntries
              .filter((entry) => entry.field === 'cacheReadPrice')
              .slice(0, 1),
          ].map((entry) => {
            const unitLabelKey = getDynamicPriceUnitLabelKey(entry)
            const unitLabel = taskUsageUnitLabel(
              entry,
              i18n.language,
              unitLabelKey ? t(unitLabelKey) : tokenUnitLabel
            )
            let label: ReactNode = null
            if (entry.labelKind !== 'schema') {
              label = t(entry.shortLabel)
            } else {
              label = taskPriceLabel(
                entry.description,
                entry.shortLabel,
                i18n.language
              )
            }
            return (
              <div
                key={entry.key}
                className={cn(
                  'flex min-w-0 flex-col gap-1',
                  dynamicSummary.isTaskUsage && 'col-span-full'
                )}
              >
                {label && (
                  <span className='text-muted-foreground text-xs whitespace-nowrap'>
                    {label}
                  </span>
                )}
                <span className='flex flex-nowrap items-baseline gap-x-1 font-mono text-[14px] font-semibold tabular-nums'>
                  <span className='whitespace-nowrap'>
                    {entry.formattedRange ?? entry.formatted}
                  </span>
                  <span className='text-muted-foreground text-xs font-normal whitespace-nowrap'>
                    {' '}
                    / {unitLabel}
                  </span>
                </span>
              </div>
            )
          })}
          {dynamicSummary.isTimePricing && timeSegments.length > 1 && (
            <div className='col-span-full flex flex-col gap-0.5'>
              {timeSegments.map((segment) => (
                <div
                  key={segment.key}
                  className='flex min-w-0 flex-wrap items-baseline justify-between gap-x-3'
                >
                  <span
                    className={cn(
                      'min-w-0 truncate text-xs',
                      segment.isCurrent
                        ? 'text-foreground font-medium'
                        : 'text-muted-foreground'
                    )}
                  >
                    {segment.label}
                    {segment.isCurrent && (
                      <>
                        {' · '}
                        <span className='text-muted-foreground font-normal'>
                          {t('Current period price')}
                        </span>
                      </>
                    )}
                  </span>
                  <span className='text-muted-foreground shrink-0 font-mono text-xs tabular-nums'>
                    {segment.prices}
                  </span>
                </div>
              ))}
            </div>
          )}
          {dynamicSummary.isTimePricing && timeSegments.length <= 1 && (
            <span className='text-muted-foreground col-span-full text-xs'>
              {t('Current period price')}
            </span>
          )}
          {dynamicSummary.isMixedBilling && (
            <span className='text-muted-foreground col-span-full text-xs'>
              {t('Token or per-call pricing')}
            </span>
          )}
          {cardExamplePrice && (
            <span className='text-muted-foreground col-span-full text-xs break-words'>
              {cardExamplePrice.label} ≈ {cardExamplePrice.formatted}
            </span>
          )}
          {dynamicSummary.isTaskUsage &&
            dynamicSummary.tier?.label &&
            !dynamicSummary.primaryEntries.some(
              (entry) => entry.formattedRange
            ) && (
              <span className='text-muted-foreground col-span-full text-xs break-words'>
                ({dynamicSummary.tier.label})
              </span>
            )}
        </>
      )
    } else {
      priceSummary = (
        <span className='text-muted-foreground col-span-full'>
          {dynamicSummary.hasUnconfiguredProviders
            ? t('Usage-based billing · price not configured')
            : t('Dynamic Pricing')}
        </span>
      )
    }
  } else if (isUnconfiguredTaskUsage) {
    priceSummary = (
      <span className='text-muted-foreground col-span-full'>
        {t('Usage-based billing · price not configured')}
      </span>
    )
  } else if (pricedPerToken) {
    const prices: { type: PriceType; label: string }[] = [
      { type: 'input', label: t('Input') },
      { type: 'output', label: t('Output') },
      ...(props.model.cache_ratio != null
        ? [{ type: 'cache' as const, label: t('Cache Read') }]
        : []),
    ]
    priceSummary = prices.map((price) => (
      <div key={price.type} className='flex min-w-0 flex-col gap-1'>
        <span className='text-muted-foreground truncate text-xs whitespace-nowrap'>
          {price.label}
          {price.type === 'cache' && (
            <span className='sr-only'>{t('Cached')}</span>
          )}
        </span>
        <span className='font-mono text-[14px] font-semibold whitespace-nowrap tabular-nums'>
          {formatPrice(
            props.model,
            price.type,
            tokenUnit,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup,
            true,
            props.groupPricingOverrides
          )}
          <span className='text-muted-foreground text-xs font-normal'>
            {' '}
            / {tokenUnitLabel}
          </span>
        </span>
      </div>
    ))
  } else {
    // Reached when the model bills per request, and also when a group pinned a
    // token-priced model to per-request billing. formatRequestPrice reads the
    // model's own per-call price, which is what such a group charges.
    priceSummary = (
      <div className='col-span-full flex min-w-0 flex-col gap-1'>
        <span className='font-mono text-[14px] font-semibold tabular-nums'>
          {formatRequestPrice(
            props.model,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup,
            undefined,
            props.groupBillingSource,
            props.groupPricingOverrides
          )}
          <span className='text-muted-foreground text-xs font-normal'>
            {' '}
            / {t('request')}
          </span>
        </span>
      </div>
    )
  }

  return (
    <Card
      size='sm'
      className={cn(
        // Translucent so the hero photo behind the page reads through the
        // grid, matching the frosted panels elsewhere on the public pages.
        'bg-card/80 hover:bg-card/95 hover:ring-foreground/20 h-full min-w-0 backdrop-blur-sm transition-colors'
      )}
    >
      <CardHeader className='flex flex-row items-start gap-2.5'>
        <div
          aria-hidden
          className='bg-muted/50 flex size-8 shrink-0 items-center justify-center rounded-md'
        >
          {modelIcon || (
            <span className='text-muted-foreground text-sm font-bold'>
              {initial}
            </span>
          )}
        </div>
        <div className='min-w-0 flex-1'>
          <h3
            className='line-clamp-1 font-mono text-[14px] leading-snug font-semibold [overflow-wrap:anywhere]'
            title={props.model.model_name}
          >
            {props.model.model_name}
          </h3>
          {props.model.vendor_name && (
            <p
              className='text-muted-foreground mt-0.5 truncate text-xs'
              title={props.model.vendor_name}
            >
              {props.model.vendor_name}
            </p>
          )}
        </div>
        <CopyButton
          value={props.model.model_name}
          tooltip={t('Copy model name')}
          className='size-7'
          iconClassName='size-3.5'
        />
      </CardHeader>
      <CardContent className='flex flex-1 flex-col gap-2.5'>
        {/* The description and tags lead, because they tell a reader what the
            model is before the numbers tell them what it costs. */}
        <div
          role='group'
          aria-label={t('Pricing')}
          className='flex min-w-0 flex-col gap-1.5'
        >
          {dynamicSummary?.providerCount && (
            <span className='text-muted-foreground text-xs break-words'>
              {t('{{count}} providers', {
                count: dynamicSummary.providerCount,
              })}
              {dynamicSummary.hasUnconfiguredProviders &&
                ` · ${t('Not configured for some providers')}`}
            </span>
          )}
          <div className='grid grid-cols-[repeat(3,minmax(0,1fr))] gap-x-3 gap-y-2'>
            {priceSummary}
          </div>
          {/* How the price above is charged, beside it rather than in the
              metadata block: "0.012 / request" only means something next to
              "per request", and the mode a group overrides is part of the
              price, not a footnote. */}
          {perRequestOverride && (
            <span className='text-muted-foreground text-xs'>
              {t('Billed per request in this group')}
            </span>
          )}
        </div>
        {/* Only the first value of each list is shown. A model reachable from
            several groups or endpoints otherwise turns its card into a wall of
            chips, and the overflow count carries the rest. */}
        <dl
          className={cn(
            'grid min-w-0 grid-cols-2 gap-x-3 gap-y-2 text-xs',
            (groups.length === 0 || endpoints.length === 0) && 'grid-cols-1'
          )}
        >
          <div className='flex min-w-0 items-baseline gap-1.5'>
            <dd className='flex min-w-0 items-baseline gap-1'>
              <ModelBillingModeBadge model={props.model} appearance='caption' />
            </dd>
          </div>
          {groups.length > 0 && (
            <div className='flex min-w-0 items-baseline gap-1.5'>
              <dt className='text-muted-foreground shrink-0'>{t('Groups')}</dt>
              <dd className='flex min-w-0 items-baseline gap-1'>
                <span className='truncate' title={groups.join(', ')}>
                  {groups[0]}
                </span>
                {groups.length > 1 && (
                  <span
                    className='text-muted-foreground shrink-0'
                    title={groups.slice(1).join(', ')}
                  >
                    +{groups.length - 1}
                  </span>
                )}
              </dd>
            </div>
          )}
          {endpoints.length > 0 && (
            <div className='flex min-w-0 items-baseline gap-1.5'>
              <dt className='text-muted-foreground shrink-0'>
                {t('Endpoints')}
              </dt>
              <dd className='flex min-w-0 items-baseline gap-1'>
                <span className='truncate' title={endpoints.join(', ')}>
                  {endpoints.slice(0, 2).join(', ')}
                </span>
                {endpoints.length > 2 && (
                  <span
                    className='text-muted-foreground shrink-0'
                    title={endpoints.slice(2).join(', ')}
                  >
                    +{endpoints.length - 2}
                  </span>
                )}
              </dd>
            </div>
          )}
          {/* The description and tags trail the numbers: the price is what a
            reader scans for, and the prose is what they read once a candidate
            is worth a second look. */}
          <div className='mt-auto flex min-w-0 flex-col gap-1.5'>
            <p className='text-muted-foreground line-clamp-1 text-xs leading-4 break-words'>
              {props.model.description || t('No description available.')}
            </p>
            {tags.length > 0 && (
              <div
                role='group'
                aria-label={t('Tags')}
                className='text-muted-foreground flex min-w-0 items-baseline gap-1.5 text-xs'
              >
                <span className='shrink-0'>{t('Tags')}</span>
                <span className='truncate' title={tags.join(', ')}>
                  {tags.slice(0, 2).join(', ')}
                </span>
                {tags.length > 2 && (
                  <span className='shrink-0' title={tags.slice(2).join(', ')}>
                    +{tags.length - 2}
                  </span>
                )}
              </div>
            )}
          </div>
        </dl>
      </CardContent>
      <CardFooter className='mt-auto border-0 bg-transparent pt-0'>
        <ModelPerfBadge perf={props.perf} className='pt-0.5'>
          <Button variant='ghost' size='sm' onClick={props.onClick}>
            {t('Details')}
            <ChevronRight aria-hidden className='size-3.5' />
          </Button>
        </ModelPerfBadge>
      </CardFooter>
    </Card>
  )
})
