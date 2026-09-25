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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'
import { cn } from '@/lib/utils'

import { AuthTrustStrip } from './components/auth-trust-strip'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading, appearance } = useSystemConfig()
  const backgroundImage = appearance.backgroundImage
  const blurOpacity = Math.min(
    100,
    Math.max(0, appearance.backgroundBlurOpacity ?? 40)
  )
  const surfaceStyles = backgroundImage
    ? ({
        '--console-card-opacity': `${Math.min(85, blurOpacity + 15)}%`,
      } as React.CSSProperties)
    : undefined
  const glassSurfaceStyle = backgroundImage
    ? {
        backgroundColor:
          'color-mix(in oklch, var(--card) var(--console-card-opacity), transparent)',
      }
    : undefined

  return (
    <div
      className='relative isolate grid min-h-svh max-w-none overflow-x-clip'
      data-console-background={backgroundImage ? 'image' : undefined}
      style={surfaceStyles}
    >
      {backgroundImage ? (
        <div
          aria-hidden='true'
          className='pointer-events-none fixed inset-0 -z-10 bg-cover bg-center bg-no-repeat'
          style={{ backgroundImage: `url(${JSON.stringify(backgroundImage)})` }}
        />
      ) : (
        <>
          {/* Same visual language as the landing hero: horizon glow, faint
              grid fading into a radial mask, and a vignette that keeps the
              edges quiet so the form owns the viewport. These three tokens
              (--primary, --muted-foreground, --background) are global, so no
              landing scope leaks into the rest of the app. */}
          <div
            aria-hidden='true'
            className='pointer-events-none absolute top-[-16rem] left-1/2 -z-10 h-[38rem] w-[120%] -translate-x-1/2 rounded-full bg-[radial-gradient(closest-side,color-mix(in_oklch,var(--primary)_22%,transparent),transparent)]'
          />
          <div
            aria-hidden='true'
            className='pointer-events-none absolute inset-0 -z-10 [background-image:linear-gradient(color-mix(in_oklch,var(--muted-foreground)_7%,transparent)_1px,transparent_1px),linear-gradient(90deg,color-mix(in_oklch,var(--muted-foreground)_7%,transparent)_1px,transparent_1px)] [mask-image:radial-gradient(ellipse_75%_60%_at_50%_35%,black,transparent)] [background-size:56px_56px]'
          />
          <div
            aria-hidden='true'
            className='pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(ellipse_90%_70%_at_50%_45%,transparent_55%,var(--background)_100%)]'
          />
        </>
      )}
      <Link
        to='/'
        className={cn(
          'absolute top-4 left-4 z-10 flex items-center gap-2 transition-opacity hover:opacity-80 sm:top-8 sm:left-8',
          backgroundImage && 'rounded-lg p-2 backdrop-blur-sm'
        )}
        style={glassSurfaceStyle}
      >
        <div className='relative h-8 max-w-[12rem]'>
          {loading ? (
            <Skeleton className='h-full w-24 rounded-md' />
          ) : (
            <img
              src={logo}
              alt={t('Logo')}
              className='h-full w-auto rounded-md object-contain'
            />
          )}
        </div>
        {loading ? (
          <Skeleton className='h-6 w-24' />
        ) : (
          <h1 className='text-xl font-medium'>{systemName}</h1>
        )}
      </Link>
      <div className='relative z-0 container flex items-center pt-16 sm:pt-0'>
        <div
          className={cn(
            'mx-auto flex w-full flex-col justify-center space-y-2 px-4 py-8 sm:w-[520px] sm:p-8',
            backgroundImage
              ? 'rounded-lg backdrop-blur-sm'
              : 'sm:rounded-2xl sm:border sm:bg-card sm:shadow-[0_18px_50px_-24px_rgba(0,0,0,0.35)]'
          )}
          style={glassSurfaceStyle}
        >
          {children}
          <AuthTrustStrip className='pt-4' />
        </div>
      </div>
    </div>
  )
}
