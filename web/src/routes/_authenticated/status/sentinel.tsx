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
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { SettingsPageProvider } from '@/features/system-settings/components/settings-page-context'
import {
  getOptionValue,
  useSystemOptions,
} from '@/features/system-settings/hooks/use-system-options'
import {
  PriceSentinelSection,
  type PriceSentinelSettingsValues,
} from '@/features/system-settings/operations/price-sentinel-section'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/status/sentinel')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: SentinelPage,
})

// This page renders the section outside the settings shell, so it loads the
// option keys the section writes itself.
const SENTINEL_SETTINGS_DEFAULTS: PriceSentinelSettingsValues = {
  'price_sentinel_setting.enabled': true,
  'price_sentinel_setting.interval_minutes': 60,
  'price_sentinel_setting.send_time': '',
  'price_sentinel_setting.report_email': '',
}

function SentinelPage() {
  const { t } = useTranslation()
  const optionsQuery = useSystemOptions()
  const defaultValues = getOptionValue(
    optionsQuery.data?.data,
    SENTINEL_SETTINGS_DEFAULTS
  ) as PriceSentinelSettingsValues

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='truncate'>{t('Upstream price sentinel')}</span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <SettingsPageProvider actionsContainer={null}>
          {optionsQuery.isLoading ? (
            // The form seeds from these values on mount, so it waits for them
            // rather than rendering defaults that would then not update.
            <LoadingState />
          ) : (
            <PriceSentinelSection defaultValues={defaultValues} />
          )}
        </SettingsPageProvider>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
