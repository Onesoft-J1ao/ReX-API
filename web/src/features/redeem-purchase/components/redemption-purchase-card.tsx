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
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { submitPaymentForm } from '@/features/wallet/lib/payment'
import { handleServerError } from '@/lib/handle-server-error'

import {
  type RedemptionPurchaseOptions,
  getRedemptionPurchaseAmount,
  getRedemptionPurchaseOptions,
  requestRedemptionPurchase,
} from '../api'

type RedemptionPurchaseCardProps = {
  onPurchaseSuccess?: () => void
}

/**
 * Buy redemption codes with real money.
 *
 * The payment methods come from the server rather than being hardcoded here, so
 * the card can never offer a method this deployment would reject -- and adding a
 * provider later needs no change on this side.
 */
export function RedemptionPurchaseCard(props: RedemptionPurchaseCardProps) {
  const { t } = useTranslation()
  const [options, setOptions] = useState<RedemptionPurchaseOptions | null>(null)
  const [unitAmount, setUnitAmount] = useState('')
  const [quantity, setQuantity] = useState('1')
  const [method, setMethod] = useState('')
  const [price, setPrice] = useState<string | null>(null)
  const [loadingPrice, setLoadingPrice] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    let cancelled = false
    void getRedemptionPurchaseOptions()
      .then((result) => {
        if (cancelled) return
        setOptions(result)
        if (result.methods.length > 0) setMethod(result.methods[0])
        setUnitAmount(String(result.min_amount > 0 ? result.min_amount : 10))
      })
      .catch(() => {
        // The feature is optional; a failed lookup just hides the card.
        if (!cancelled) setOptions(null)
      })
    return () => {
      cancelled = true
    }
  }, [])

  const payload = useCallback(
    () => ({
      unit_amount: Number(unitAmount),
      quantity: Number(quantity),
      payment_method: method,
    }),
    [unitAmount, quantity, method]
  )

  // Price is server-decided (per-provider pricing and group discounts live
  // there), so it is re-asked whenever the order changes.
  useEffect(() => {
    if (!options?.enabled || !method || !unitAmount || !quantity) return
    const amount = Number(unitAmount)
    const count = Number(quantity)
    if (!Number.isFinite(amount) || amount <= 0 || !Number.isFinite(count) || count <= 0) {
      setPrice(null)
      return
    }
    let cancelled = false
    setLoadingPrice(true)
    const timer = window.setTimeout(() => {
      void getRedemptionPurchaseAmount(payload())
        .then((value) => {
          if (!cancelled) setPrice(value)
        })
        .catch(() => {
          if (!cancelled) setPrice(null)
        })
        .finally(() => {
          if (!cancelled) setLoadingPrice(false)
        })
    }, 300)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [options?.enabled, method, unitAmount, quantity, payload])

  if (!options?.enabled || options.methods.length === 0) {
    return null
  }

  const handlePurchase = async () => {
    setSubmitting(true)
    try {
      const result = await requestRedemptionPurchase(payload())
      if (result.message !== 'success') {
        toast.error(String(result.data ?? t('Payment request failed')))
        return
      }
      if (result.url) {
        // epay hands back form parameters plus the endpoint to post them to.
        submitPaymentForm(result.url, (result.data ?? {}) as Record<string, unknown>)
      } else {
        const payLink = (result.data as { pay_link?: string } | undefined)?.pay_link
        if (!payLink) {
          toast.error(t('Payment request failed'))
          return
        }
        window.open(payLink, '_blank')
      }
      toast.success(t('Payment page opened'))
      props.onPurchaseSuccess?.()
    } catch (error) {
      handleServerError(error, t('Payment request failed'))
    } finally {
      setSubmitting(false)
    }
  }

  let priceLabel = t('Enter a face value and quantity')
  if (price !== null) {
    priceLabel = `${t('Pay')} ${price}`
  } else if (loadingPrice) {
    priceLabel = t('Calculating...')
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Buy redemption codes')}</CardTitle>
        <CardDescription>
          {t(
            'Pay with a payment method and receive redemption codes of the face value you choose.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-end'>
          <div className='flex flex-1 flex-col gap-1.5'>
            <Label htmlFor='redemption-unit-amount'>{t('Face value')}</Label>
            <Input
              id='redemption-unit-amount'
              type='number'
              min={1}
              value={unitAmount}
              onChange={(event) => setUnitAmount(event.target.value)}
            />
          </div>
          <div className='flex flex-1 flex-col gap-1.5'>
            <Label htmlFor='redemption-quantity'>{t('Quantity')}</Label>
            <Input
              id='redemption-quantity'
              type='number'
              min={1}
              max={options.max_count}
              value={quantity}
              onChange={(event) => setQuantity(event.target.value)}
            />
          </div>
          <div className='flex flex-1 flex-col gap-1.5'>
            <Label htmlFor='redemption-method'>{t('Payment method')}</Label>
            <Select
              value={method}
              onValueChange={(value) => setMethod(value ?? '')}
            >
              <SelectTrigger id='redemption-method'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {options.methods.map((candidate) => (
                  <SelectItem key={candidate} value={candidate}>
                    {candidate}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className='flex items-center justify-between gap-3'>
          <span className='text-muted-foreground text-sm'>{priceLabel}</span>
          <Button
            onClick={() => void handlePurchase()}
            disabled={submitting || price === null}
          >
            {submitting ? t('Processing...') : t('Buy now')}
          </Button>
        </div>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Up to {{count}} codes per order. Codes are delivered after the payment is confirmed.',
            { count: options.max_count }
          )}
        </p>
      </CardContent>
    </Card>
  )
}
