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

import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useUnsplashBackground } from '@/hooks/use-unsplash-background'
import { cn } from '@/lib/utils'
import { pickLocalBackground } from '@/lib/local-backgrounds'

import { consoleSurfaceStyles } from '../lib/console-surface'
import type { TopNavLink } from '../types'
import { BackgroundCredit } from './background-credit'
import { PublicHeader, type PublicHeaderProps } from './public-header'

type PublicLayoutProps = {
  children: React.ReactNode
  showMainContainer?: boolean
  navContent?: React.ReactNode
  headerProps?: Omit<PublicHeaderProps, 'navContent'>
  navLinks?: TopNavLink[]
  showThemeSwitch?: boolean
  showAuthButtons?: boolean
  showNotifications?: boolean
  logo?: React.ReactNode
  siteName?: string
  backgroundMode?: 'none' | 'hero'
  /** Wins over both the configured image and the bundled pool when non-empty. */
  backgroundOverride?: string
  /**
   * A page-specific image is still being resolved. The backdrop stays empty
   * instead of showing the bundled pool and swapping it out a moment later.
   */
  backgroundPending?: boolean
}

export function PublicLayout(props: PublicLayoutProps) {
  const { appearance } = useSystemConfig()
  const { backgroundOpacity } = useThemeCustomization()
  // Precedence: a page-provided image (the anime pages pass a Nekosia pick),
  // then the operator's explicit image, then Unsplash when a query is
  // configured, then the bundled pool. The local pick is fixed at mount so a
  // re-render cannot swap the picture.
  const [localBackground] = useState(pickLocalBackground)
  const {
    image: unsplashImage,
    credit,
    pending: unsplashPending,
  } = useUnsplashBackground(!props.backgroundOverride)
  const backgroundImage = props.backgroundPending
    ? props.backgroundOverride
    : props.backgroundOverride ||
      appearance.backgroundImage ||
      unsplashImage ||
      // Hold the backdrop empty while the lookup runs instead of flashing the
      // bundled pool and swapping it a moment later.
      (unsplashPending ? '' : localBackground)
  // The Unsplash credit only applies when that image is the one on screen.
  const showCredit =
    !props.backgroundOverride && !appearance.backgroundImage && !!unsplashImage
  const showHeroBackground = props.backgroundMode === 'hero' && !!backgroundImage

  return (
    <div
      className='bg-background text-foreground relative min-h-svh overflow-x-clip'
      data-console-background={showHeroBackground ? 'image' : undefined}
      style={
        showHeroBackground
          ? consoleSurfaceStyles(
              backgroundImage,
              appearance.backgroundBlurOpacity,
              backgroundOpacity
            )
          : undefined
      }
    >
      {showHeroBackground && (
        <div className='pointer-events-none fixed inset-0 overflow-hidden'>
          <div
            aria-hidden='true'
            className='absolute inset-0 bg-cover bg-center bg-no-repeat'
            style={{
              backgroundImage: `url(${JSON.stringify(backgroundImage)})`,
            }}
          />
          {/* A gentle wash keeps text legible over the photo without erasing
              it: the backdrop stays visible behind every translucent surface
              all the way down the page, which is what makes the frosted
              panels read as frosted. */}
          <div
            aria-hidden='true'
            className='from-background/30 via-background/55 to-background/75 absolute inset-0 bg-linear-to-b'
          />
          {showCredit && credit && <BackgroundCredit credit={credit} />}
        </div>
      )}
      <PublicHeader
        navContent={props.navContent}
        navLinks={props.navLinks}
        showThemeSwitch={props.showThemeSwitch}
        showAuthButtons={props.showAuthButtons}
        showNotifications={props.showNotifications}
        logo={props.logo}
        siteName={props.siteName}
        {...props.headerProps}
        className={cn(
          showHeroBackground && 'border-0 bg-transparent',
          props.headerProps?.className
        )}
      />

      {props.showMainContainer !== false ? (
        <main className='relative container px-4 py-6 pt-20 md:px-4'>
          {props.children}
        </main>
      ) : (
        props.children
      )}
    </div>
  )
}
