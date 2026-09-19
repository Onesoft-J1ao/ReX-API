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
import { Move } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionTitle } from '@/components/section-title'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { WALLPAPER_ERROR_MESSAGES } from '@/features/wallpaper/constants'
import {
  useClearWallpaper,
  useResolveWallpaperSource,
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
import {
  WALLPAPER_VISIBILITY_MAX,
  WALLPAPER_VISIBILITY_MIN,
} from '@/lib/theme-customization'
import { useAuthStore } from '@/stores/auth-store'

import { FramingSlider } from './framing-slider'
import { WallpaperFraming } from './wallpaper-framing'

/**
 * Wallpaper section of the appearance drawer. The image itself is painted by
 * `ThemeCustomizationProvider`; this component only owns the paid change —
 * choosing an address, quoting the price, and spending one of the month's
 * changes once the user confirms.
 */
export function WallpaperConfig() {
  const { t } = useTranslation()
  const {
    wallpaperUrl,
    wallpaperFraming,
    wallpaperAccentEnabled,
    wallpaperVisibility,
    setWallpaperFraming,
    setWallpaperAccentEnabled,
    setWallpaperVisibility,
    setWallpaperFramingMode,
  } = useThemeCustomization()
  const { data: wallpaper } = useWallpaper()
  const { status: systemStatus } = useStatus()
  const balance = useAuthStore((state) => state.auth.user?.quota)
  const saveWallpaper = useUpdateWallpaper()
  const removeWallpaper = useClearWallpaper()
  const resolveSource = useResolveWallpaperSource()

  const [draft, setDraft] = useState('')
  const [sourceDraft, setSourceDraft] = useState('')
  // Set when the resolved item is a video: Steam publishes no video for one, so
  // the console says why the address it filled in is only a still.
  const [resolvedVideoTitle, setResolvedVideoTitle] = useState<string | null>(
    null
  )
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

  // Resolving only fills the address field: the change itself still goes through
  // the paid confirmation below.
  const handleResolve = () => {
    resolveSource.mutate(sourceDraft.trim(), {
      onSuccess: (response) => {
        const result = response.data
        if (!result) {
          toast.error(t('Failed to resolve the link'))
          return
        }
        setDraft(result.image_url)
        setResolvedVideoTitle(result.is_video ? result.title : null)
        toast.success(
          result.wallpaper_engine
            ? t('Resolved "{{title}}". Press Change wallpaper to use it.', {
                title: result.title,
              })
            : t('Resolved "{{title}}". It is not a Wallpaper Engine item.', {
                title: result.title,
              })
        )
      },
      onError: (error) => {
        reportFailure(error, t('Failed to resolve the link'))
      },
    })
  }

  return (
    <div>
      <SectionTitle title={t('Custom wallpaper')} />

      {wallpaperUrl !== '' && (
        <>
          <WallpaperFraming url={wallpaperUrl} />

          <div className='mb-3 space-y-2'>
            <Button
              size='sm'
              variant='outline'
              className='h-7 px-2 text-xs'
              onClick={() => setWallpaperFramingMode(true)}
            >
              <Move className='mr-1.5 size-3' aria-hidden='true' />
              {t('Drag on the page to frame')}
            </Button>

            <div className='flex items-center gap-2'>
              <span className='text-muted-foreground w-14 shrink-0 text-xs'>
                {t('Size')}
              </span>
              <Button
                size='sm'
                variant={wallpaperFraming.fit === 'cover' ? 'secondary' : 'ghost'}
                className='h-7 px-2 text-xs'
                onClick={() =>
                  setWallpaperFraming({ ...wallpaperFraming, fit: 'cover' })
                }
              >
                {t('Fill the screen')}
              </Button>
              <Button
                size='sm'
                variant={
                  wallpaperFraming.fit === 'contain' ? 'secondary' : 'ghost'
                }
                className='h-7 px-2 text-xs'
                onClick={() =>
                  setWallpaperFraming({ ...wallpaperFraming, fit: 'contain' })
                }
              >
                {t('Show the whole picture')}
              </Button>
            </div>

            <FramingSlider
              label={t('Visibility')}
              value={wallpaperVisibility}
              min={WALLPAPER_VISIBILITY_MIN}
              max={WALLPAPER_VISIBILITY_MAX}
              step={5}
              format={(value) => `${Math.round(value)}%`}
              onChange={setWallpaperVisibility}
            />
            <p className='text-muted-foreground text-xs'>
              {t(
                'How much of the wallpaper shows through the console panels. Higher means a clearer picture and busier panels.'
              )}
            </p>
          </div>

          <div className='mb-3 flex items-start justify-between gap-3 rounded-md border p-3'>
            <div className='space-y-0.5'>
              <Label htmlFor='wallpaper-accent' className='text-xs'>
                {t('Match the theme colour to the wallpaper')}
              </Label>
              <p className='text-muted-foreground text-xs'>
                {t(
                  "Takes the accent colour from the picture's dominant colour. If the host does not allow the picture to be read, the preset's own accent stays."
                )}
              </p>
            </div>
            <Switch
              id='wallpaper-accent'
              checked={wallpaperAccentEnabled}
              onCheckedChange={setWallpaperAccentEnabled}
            />
          </div>
        </>
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

      <div className='mb-3'>
        <Label htmlFor='wallpaper-source' className='mb-1 block text-xs'>
          {t('Wallpaper Engine link')}
        </Label>
        <div className='flex items-center gap-2'>
          <Input
            id='wallpaper-source'
            value={sourceDraft}
            onChange={(event) => setSourceDraft(event.target.value)}
            placeholder='https://steamcommunity.com/sharedfiles/filedetails/?id=3778060647'
            aria-describedby='wallpaper-source-description'
          />
          <Button
            size='sm'
            variant='outline'
            className='shrink-0'
            onClick={handleResolve}
            disabled={sourceDraft.trim() === '' || resolveSource.isPending}
          >
            {resolveSource.isPending ? t('Resolving...') : t('Resolve')}
          </Button>
        </div>
        <p
          id='wallpaper-source-description'
          className='text-muted-foreground mt-1 text-xs'
        >
          {t(
            'Paste a Steam Workshop link and its preview image fills the address below.'
          )}
        </p>
        {resolvedVideoTitle !== null && (
          <p role='status' className='text-muted-foreground mt-1 text-xs'>
            {t(
              '“{{title}}” is a video wallpaper, so Steam publishes only a still preview. Paste a video address (.mp4 / .webm) if you want motion.',
              { title: resolvedVideoTitle }
            )}
          </p>
        )}
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
        )}{' '}
        {t('Video addresses (.mp4 / .webm) play as an animated background.')}
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
