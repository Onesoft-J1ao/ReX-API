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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionTitle } from '@/components/section-title'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { WALLPAPER_ERROR_MESSAGES } from '@/features/wallpaper/constants'
import {
  useClearWallpaper,
  useUpdateWallpaper,
  useWallpaper,
} from '@/features/wallpaper/hooks/use-wallpaper'
import {
  formatWallpaperPrice,
  getWallpaperErrorCode,
  normalizeWallpaperUrl,
  wallpaperPriceInQuota,
} from '@/features/wallpaper/lib/wallpaper'
import { useStatus } from '@/hooks/use-status'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

/**
 * Wallpaper section of the appearance drawer. The image itself is painted by
 * `ThemeCustomizationProvider`; this component only owns the paid change —
 * choosing an address, quoting the price, and spending one of the month's
 * changes once the user confirms.
 */
export function WallpaperConfig() {
  const { t } = useTranslation()
  const { wallpaperUrl } = useThemeCustomization()
  const { data: wallpaper } = useWallpaper()
  const { status: systemStatus } = useStatus()
  const balance = useAuthStore((state) => state.auth.user?.quota)
  const saveWallpaper = useUpdateWallpaper()
  const removeWallpaper = useClearWallpaper()

  const [draft, setDraft] = useState('')
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [confirmingRemove, setConfirmingRemove] = useState(false)

  if (!wallpaper?.enabled) return null

  const normalizedDraft = normalizeWallpaperUrl(draft)
  const unlimited = wallpaper.monthly_limit <= 0
  const remaining = wallpaper.changes_remaining
  const outOfChanges = !unlimited && remaining <= 0
  const priceQuota = wallpaperPriceInQuota(
    wallpaper.price_yuan,
    systemStatus?.usd_exchange_rate ?? 0,
    systemStatus?.quota_per_unit ?? 0
  )
  const shortOnBalance =
    priceQuota !== null && balance !== undefined && balance < priceQuota
  const canSave = Boolean(normalizedDraft) && !outOfChanges && !shortOnBalance
  const previewUrl = normalizedDraft ?? wallpaperUrl

  const reportFailure = (error: unknown, fallback: string) => {
    const code = getWallpaperErrorCode((error as { cause?: unknown })?.cause)
    if (code) {
      toast.error(t(WALLPAPER_ERROR_MESSAGES[code]))
      return
    }
    handleServerError(error, fallback)
  }

  const handleSave = () => {
    saveWallpaper.mutate(normalizedDraft ?? '', {
      onSuccess: () => {
        setConfirmingSave(false)
        setDraft('')
        toast.success(t('Wallpaper updated'))
      },
      onError: (error) => {
        setConfirmingSave(false)
        reportFailure(error, t('Failed to update wallpaper'))
      },
    })
  }

  const handleRemove = () => {
    removeWallpaper.mutate(undefined, {
      onSuccess: () => {
        setConfirmingRemove(false)
        toast.success(t('Wallpaper removed'))
      },
      onError: (error) => {
        setConfirmingRemove(false)
        reportFailure(error, t('Failed to remove wallpaper'))
      },
    })
  }

  return (
    <div>
      <SectionTitle title={t('Custom wallpaper')} />

      {wallpaperUrl !== '' && (
        <div className='mb-3'>
          <img
            src={wallpaperUrl}
            alt={t('Current wallpaper')}
            referrerPolicy='no-referrer'
            loading='lazy'
            className='ring-border h-24 w-full rounded-md object-cover ring-[1px]'
          />
        </div>
      )}

      <div className='text-muted-foreground mb-3 space-y-1 text-xs'>
        <p>
          {t('One change costs {{price}}.', {
            price: formatWallpaperPrice(wallpaper.price_yuan),
          })}
        </p>
        <p>
          {unlimited
            ? t('Unlimited changes this month')
            : t('{{used}} of {{limit}} changes used this month', {
                used: wallpaper.changes_used,
                limit: wallpaper.monthly_limit,
              })}
        </p>
      </div>

      <Label htmlFor='wallpaper-url' className='mb-1 block text-xs'>
        {t('Wallpaper image address')}
      </Label>
      <Input
        id='wallpaper-url'
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        placeholder='https://example.com/wallpaper.jpg'
        aria-describedby='wallpaper-url-description'
        aria-invalid={draft !== '' && normalizedDraft === null}
        className='mb-1'
      />
      <p
        id='wallpaper-url-description'
        className='text-muted-foreground mb-3 text-xs'
      >
        {t(
          'Paste an absolute http(s) link to an image. Your console loads it from that address.'
        )}
      </p>

      {normalizedDraft !== null && (
        <img
          src={previewUrl}
          alt={t('Wallpaper preview')}
          referrerPolicy='no-referrer'
          loading='lazy'
          className='ring-border mb-3 h-24 w-full rounded-md object-cover ring-[1px]'
        />
      )}

      {outOfChanges && (
        <p role='alert' className='text-destructive mb-3 text-xs'>
          {t(WALLPAPER_ERROR_MESSAGES.WALLPAPER_MONTHLY_LIMIT_REACHED)}
        </p>
      )}
      {!outOfChanges && shortOnBalance && (
        <p role='alert' className='text-destructive mb-3 text-xs'>
          {t(WALLPAPER_ERROR_MESSAGES.WALLPAPER_INSUFFICIENT_QUOTA)}
        </p>
      )}

      <div className='flex flex-wrap gap-2'>
        <Button
          size='sm'
          onClick={() => setConfirmingSave(true)}
          disabled={!canSave || saveWallpaper.isPending}
          title={
            canSave
              ? t('Change wallpaper')
              : t('Wallpaper changes are unavailable right now')
          }
        >
          {t('Change wallpaper')}
        </Button>
        {wallpaperUrl !== '' && (
          <Button
            size='sm'
            variant='outline'
            onClick={() => setConfirmingRemove(true)}
            disabled={removeWallpaper.isPending}
          >
            {t('Remove wallpaper')}
          </Button>
        )}
      </div>

      <ConfirmDialog
        open={confirmingSave}
        onOpenChange={setConfirmingSave}
        title={t('Confirm the wallpaper change')}
        desc={
          unlimited
            ? t(
                'This change costs {{price}}. Your remaining changes are unlimited.',
                { price: formatWallpaperPrice(wallpaper.price_yuan) }
              )
            : t(
                'This change costs {{price}} and leaves {{remaining}} of {{limit}} changes for this month.',
                {
                  price: formatWallpaperPrice(wallpaper.price_yuan),
                  remaining: Math.max(remaining - 1, 0),
                  limit: wallpaper.monthly_limit,
                }
              )
        }
        confirmText={t('Change wallpaper')}
        handleConfirm={handleSave}
        isLoading={saveWallpaper.isPending}
      />

      <ConfirmDialog
        open={confirmingRemove}
        onOpenChange={setConfirmingRemove}
        title={t('Remove this wallpaper?')}
        desc={t(
          'Your console returns to the default background. Removing is free and does not give back a change.'
        )}
        confirmText={t('Remove wallpaper')}
        destructive
        handleConfirm={handleRemove}
        isLoading={removeWallpaper.isPending}
      />
    </div>
  )
}
