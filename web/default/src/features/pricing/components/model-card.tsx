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
import { ChevronRight, Copy } from 'lucide-react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { DEFAULT_TOKEN_UNIT } from '../constants'
import {
  getDynamicDisplayGroupRatio,
  getDynamicPricingSummary,
} from '../lib/dynamic-price'
import { parseTags } from '../lib/filters'
import { isTokenBasedModel } from '../lib/model-helpers'
import { formatPrice, formatRequestPrice } from '../lib/price'
import type { PricingModel, TokenUnit } from '../types'
import { ModelPerfBadge, type ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardProps {
  model: PricingModel
  onClick: () => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  perf?: ModelPerfBadgeData
}

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const isTokenBased = isTokenBasedModel(props.model)
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const tags = parseTags(props.model.tags)
  const groups = props.model.enable_groups || []
  const endpoints = props.model.supported_endpoint_types || []
  const modelIconKey = props.model.icon || props.model.vendor_icon
  const modelIcon = modelIconKey ? getLobeIcon(modelIconKey, 28) : null
  const initial = props.model.model_name?.charAt(0).toUpperCase() || '?'
  const isDynamicPricing =
    props.model.billing_mode === 'tiered_expr' &&
    Boolean(props.model.billing_expr)
  const hasCachedPrice = isTokenBased && props.model.cache_ratio != null
  const dynamicSummary = isDynamicPricing
    ? getDynamicPricingSummary(props.model, {
        tokenUnit,
        showRechargePrice,
        priceRate,
        usdExchangeRate,
        groupRatioMultiplier: getDynamicDisplayGroupRatio(props.model),
      })
    : null

  const primaryGroup = groups[0]
  const bottomTags = [...endpoints.slice(0, 2), ...tags.slice(0, 2)]
  const hiddenCount =
    Math.max(groups.length - 1, 0) +
    Math.max(endpoints.length - 2, 0) +
    Math.max(tags.length - 2, 0)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(props.model.model_name || '')
  }

  return (
    <div
      className={cn(
        'group relative flex min-h-[188px] flex-col overflow-hidden rounded-lg border bg-background/90 p-3 shadow-sm transition-all duration-200 sm:p-4',
        'before:pointer-events-none before:absolute before:inset-x-0 before:top-0 before:h-px before:bg-gradient-to-r before:from-transparent before:via-foreground/20 before:to-transparent',
        'hover:-translate-y-0.5 hover:border-foreground/20 hover:bg-background hover:shadow-md'
      )}
    >
      {/* Header: icon + name + price + actions */}
      <div className='flex items-start justify-between gap-2.5'>
        <div className='flex min-w-0 items-start gap-2.5'>
          <div className='bg-muted/40 ring-border/60 flex size-9 shrink-0 items-center justify-center rounded-lg ring-1 sm:size-10'>
            {modelIcon || (
              <span className='text-muted-foreground text-sm font-bold'>
                {initial}
              </span>
            )}
          </div>
          <div className='min-w-0'>
            <h3 className='text-foreground truncate font-mono text-[14px] leading-tight font-bold sm:text-[15px]'>
              {props.model.model_name}
            </h3>
            <div className='mt-1 flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-xs sm:gap-x-2.5'>
              {dynamicSummary ? (
                dynamicSummary.isSpecialExpression ? (
                  <span className='min-w-0'>
                    <span className='text-amber-700 dark:text-amber-300'>
                      {t('Special billing expression')}
                    </span>
                    <code className='text-muted-foreground/70 mt-0.5 line-clamp-1 block font-mono text-[11px] break-all'>
                      {dynamicSummary.rawExpression}
                    </code>
                  </span>
                ) : dynamicSummary.primaryEntries.length > 0 ? (
                  <>
                    {dynamicSummary.primaryEntries.map((entry) => (
                      <span
                        key={entry.key}
                        className='text-muted-foreground whitespace-nowrap'
                      >
                        {t(entry.shortLabel)}{' '}
                        <span className='text-foreground font-mono font-semibold'>
                          {entry.formatted}
                        </span>
                        /{tokenUnitLabel}
                      </span>
                    ))}
                  </>
                ) : (
                  <span className='text-muted-foreground text-xs'>
                    {t('Dynamic Pricing')}
                  </span>
                )
              ) : isTokenBased ? (
                <>
                  <span className='text-muted-foreground whitespace-nowrap'>
                    {t('Input')}{' '}
                    <span className='text-foreground font-mono font-semibold'>
                      {formatPrice(
                        props.model,
                        'input',
                        tokenUnit,
                        showRechargePrice,
                        priceRate,
                        usdExchangeRate
                      )}
                    </span>
                    /{tokenUnitLabel}
                  </span>
                  <span className='text-muted-foreground whitespace-nowrap'>
                    {t('Output')}{' '}
                    <span className='text-foreground font-mono font-semibold'>
                      {formatPrice(
                        props.model,
                        'output',
                        tokenUnit,
                        showRechargePrice,
                        priceRate,
                        usdExchangeRate
                      )}
                    </span>
                    /{tokenUnitLabel}
                  </span>
                  {hasCachedPrice && (
                    <span className='text-muted-foreground/60 whitespace-nowrap'>
                      {t('Cached')}{' '}
                      <span className='font-mono'>
                        {formatPrice(
                          props.model,
                          'cache',
                          tokenUnit,
                          showRechargePrice,
                          priceRate,
                          usdExchangeRate
                        )}
                      </span>
                    </span>
                  )}
                </>
              ) : (
                <span className='text-muted-foreground whitespace-nowrap'>
                  <span className='text-foreground font-mono font-semibold'>
                    {formatRequestPrice(
                      props.model,
                      showRechargePrice,
                      priceRate,
                      usdExchangeRate
                    )}
                  </span>{' '}
                  / {t('request')}
                </span>
              )}
            </div>
          </div>
        </div>

        <div className='flex shrink-0 items-center gap-1'>
          <button
            type='button'
            onClick={props.onClick}
            className='text-muted-foreground hover:text-foreground hover:bg-muted inline-flex h-7 items-center gap-1 rounded-md border bg-background/70 px-2 text-xs transition-colors'
          >
            {t('Details')}
            <ChevronRight className='size-3.5' />
          </button>
          <button
            type='button'
            onClick={handleCopy}
            className='text-muted-foreground hover:text-foreground hover:bg-muted flex size-7 items-center justify-center rounded-md border bg-background/70 transition-colors'
            title={t('Copy')}
          >
            <Copy className='size-3.5' />
          </button>
        </div>
      </div>

      {/* Description */}
      <p className='text-muted-foreground mt-3 line-clamp-2 flex-1 text-[13px] leading-relaxed sm:min-h-[2.45rem]'>
        {props.model.description || t('No description available.')}
      </p>

      {/* Footer: left metadata and right performance summary share row alignment */}
      <div className='border-border/60 mt-3 grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-1 border-t pt-3'>
        <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
          {primaryGroup && (
            <span className='text-muted-foreground text-xs font-medium'>
              {primaryGroup} {t('Groups')}
            </span>
          )}
          <span className='text-muted-foreground text-xs font-medium'>
            {isTokenBased ? t('Token-based') : t('Per Request')}
          </span>
          {isDynamicPricing && (
            <StatusBadge
              label={t('Dynamic Pricing')}
              variant='warning'
              copyable={false}
              size='sm'
            />
          )}
        </div>
        <ModelPerfBadge perf={props.perf} className='row-span-2 self-start' />

        <div className='flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-0.5 sm:gap-x-3 sm:gap-y-1'>
          {bottomTags.map((item) => (
            <span key={item} className='text-muted-foreground/70 text-xs'>
              {item}
            </span>
          ))}
          <span className='text-muted-foreground/50 text-xs'>
            {tokenUnitLabel}
          </span>
          {hiddenCount > 0 && (
            <span className='text-muted-foreground/40 text-xs'>
              +{hiddenCount}
            </span>
          )}
        </div>
      </div>
    </div>
  )
})
