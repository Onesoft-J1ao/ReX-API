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

import { NoticeDialog } from '@/components/notice-dialog'
import { AnimatedOutlet } from '@/components/page-transition'
import { SkipToMain } from '@/components/skip-to-main'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { WallpaperSync } from '@/features/wallpaper/components/wallpaper-sync'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useUnsplashBackground } from '@/hooks/use-unsplash-background'
import { getCookie } from '@/lib/cookies'
import { pickLocalBackground } from '@/lib/local-backgrounds'
import { cn } from '@/lib/utils'

import { consoleSurfaceStyles } from '../lib/console-surface'
import { AppHeader } from './app-header'
import { BackgroundCredit } from './background-credit'
import { AppSidebar } from './app-sidebar'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
}

export function AuthenticatedLayout(props: AuthenticatedLayoutProps) {
  const { appearance } = useSystemConfig()
  const { backgroundOpacity } = useThemeCustomization()
  const savedSidebarState = getCookie('sidebar_state')
  const defaultOpen = savedSidebarState
    ? savedSidebarState !== 'false'
    : appearance.defaultSidebarLayout === 'expanded'
  // An operator-set image wins, then Unsplash when configured; otherwise the
  // bundled pool keeps the console from being the one flat page on a site that
  // has a backdrop elsewhere.
  const [localBackground] = useState(pickLocalBackground)
  const {
    image: unsplashImage,
    credit,
    pending: unsplashPending,
  } = useUnsplashBackground()
  const backgroundImage =
    appearance.backgroundImage ||
    unsplashImage ||
    (unsplashPending ? '' : localBackground)
  const showCredit = !appearance.backgroundImage && !!unsplashImage
  const surfaceStyles = consoleSurfaceStyles(
    backgroundImage,
    appearance.backgroundBlurOpacity,
    backgroundOpacity
  )

  return (
    <div
      className='relative isolate min-h-svh'
      data-console-background='image'
      style={surfaceStyles}
    >
      <div
        aria-hidden='true'
        className='pointer-events-none fixed inset-0 -z-10 bg-cover bg-center bg-no-repeat'
        style={{ backgroundImage: `url(${JSON.stringify(backgroundImage)})` }}
      />
      <div
        aria-hidden='true'
        className='from-background/30 via-background/55 to-background/75 pointer-events-none fixed inset-0 -z-10 bg-linear-to-b'
      />
      {showCredit && credit && <BackgroundCredit credit={credit} />}
      <LayoutProvider>
        <SearchProvider>
          <WallpaperSync />
          <SidebarProvider
            defaultOpen={defaultOpen}
            className='flex-col has-data-[variant=inset]:bg-transparent'
          >
            <SkipToMain />
            <AppHeader
              className='backdrop-blur-sm'
              style={{
                backgroundColor:
                  'color-mix(in oklch, var(--background) var(--console-surface-opacity), transparent)',
              }}
            />
            <NoticeDialog />
            <div className='flex min-h-0 w-full flex-1'>
              <AppSidebar />
              <SidebarInset
                className={cn(
                  '@container/content',
                  'h-[calc(100svh-var(--app-header-height,0px))]',
                  'min-h-0 overflow-hidden',
                  'pb-0',
                  'backdrop-blur-sm',
                  'peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]'
                )}
                style={{
                  backgroundColor:
                    'color-mix(in oklch, var(--background) var(--console-surface-opacity), transparent)',
                }}
              >
                {props.children ?? <AnimatedOutlet />}
              </SidebarInset>
            </div>
          </SidebarProvider>
        </SearchProvider>
      </LayoutProvider>
    </div>
  )
}
