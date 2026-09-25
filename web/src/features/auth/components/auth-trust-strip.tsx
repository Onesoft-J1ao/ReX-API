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

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { getPricing } from '@/features/pricing/api'
import { useStatus } from '@/hooks/use-status'
import { formatCompactNumber } from '@/lib/format'
import { toIntlLocale } from '@/i18n/languages'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

/**
 * One quiet line under the auth card: how many models the site serves and how
 * many ways in exist. Anonymous-safe by construction — pricing is public unless
 * the operator locks it behind login, in which case the whole strip hides
 * instead of erroring.
 */
export function AuthTrustStrip({ className }: { className?: string }) {
  const { t, i18n } = useTranslation()
  const { status } = useStatus()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  const pricingQuery = useQuery({
    queryKey: ['pricing'],
    queryFn: async () => requireServerSuccess(await getPricing()),
    staleTime: 5 * 60 * 1000,
    retry: false,
  })
  if (pricingQuery.isError) return null
  const modelCount = pricingQuery.data?.data?.length ?? 0
  if (modelCount === 0) return null

  const waysIn = [
    status?.password_login_enabled !== false ? t('Password') : null,
    status?.github_oauth ? 'GitHub' : null,
    status?.discord_oauth ? 'Discord' : null,
    status?.linuxdo_oauth ? 'LinuxDO' : null,
    status?.telegram_oauth ? 'Telegram' : null,
    status?.wechat_login ? t('WeChat') : null,
    status?.passkey_login ? t('Passkey') : null,
    (status?.custom_oauth_providers?.length ?? 0) > 0 ? t('SSO') : null,
  ].filter((way): way is string => way !== null)

  return (
    <p className={cn('text-muted-foreground text-center text-xs', className)}>
      {t('This site currently has {{count}} models enabled', {
        count: formatCompactNumber(modelCount, locale),
      })}
      {waysIn.length > 1 && (
        <>
          {' · '}
          {t('{{ways}} sign-in', { ways: waysIn.join(' / ') })}
        </>
      )}
    </p>
  )
}
