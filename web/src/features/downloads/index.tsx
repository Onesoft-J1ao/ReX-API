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
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import { ToolCard } from './components/tool-card'
import {
  buildDownloadCatalog,
  PLATFORM_LABEL_KEYS,
  SELECTABLE_PLATFORMS,
} from './constants'
import { useClientTarget } from './hooks/use-client-target'
import {
  CC_SWITCH_FALLBACK_RELEASE,
  resolveCcSwitchRelease,
  resolveManifestUrl,
} from './lib/cc-switch'
import type { ClientPlatform } from './types'

export function Downloads() {
  const { t } = useTranslation()
  const detected = useClientTarget()
  const [platformOverride, setPlatformOverride] =
    useState<ClientPlatform | null>(null)

  const release = useQuery({
    queryKey: ['downloads', 'cc-switch-release'],
    queryFn: () => resolveCcSwitchRelease({ manifestUrl: resolveManifestUrl() }),
    staleTime: 30 * 60 * 1000,
  })

  const catalog = useMemo(
    () =>
      buildDownloadCatalog(release.data ?? CC_SWITCH_FALLBACK_RELEASE, {
        resolving: release.isPending,
      }),
    [release.data, release.isPending]
  )

  const activePlatform = platformOverride ?? detected.platform
  const target = useMemo(
    () => ({ platform: activePlatform, arch: detected.arch }),
    [activePlatform, detected.arch]
  )

  const statusMessage =
    activePlatform === 'unknown'
      ? t(
          'Your system could not be detected. Choose your platform to highlight the matching downloads.'
        )
      : t('Downloads for {{platform}} are highlighted.', {
          platform: t(PLATFORM_LABEL_KEYS[activePlatform]),
        })

  const toggleValues = SELECTABLE_PLATFORMS.includes(activePlatform)
    ? [activePlatform]
    : []

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Tool Downloads')}</SectionPageLayout.Title>

      <SectionPageLayout.Actions>
        <ToggleGroup
          value={toggleValues}
          onValueChange={(values) => {
            const next = values[0]
            if (SELECTABLE_PLATFORMS.includes(next as ClientPlatform)) {
              setPlatformOverride(next as ClientPlatform)
            }
          }}
          variant='outline'
          size='sm'
          aria-label={t('Select your platform')}
        >
          {SELECTABLE_PLATFORMS.map((platform) => (
            <ToggleGroupItem key={platform} value={platform}>
              {t(PLATFORM_LABEL_KEYS[platform])}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </SectionPageLayout.Actions>

      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-4xl flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>{statusMessage}</p>

          {catalog.map((tool) => (
            <ToolCard key={tool.id} tool={tool} target={target} />
          ))}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
