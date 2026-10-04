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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { toIntlLocale } from '@/i18n/languages'
import {
  formatCompactNumber,
  formatNumber,
  formatTimestampToDate,
} from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ApiKey, ApiKeyIpStat } from '../types'

type ApiKeyIpCellProps = {
  apiKey: ApiKey
  stat: ApiKeyIpStat | undefined
}

export function ApiKeyIpCell(props: ApiKeyIpCellProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const stat = props.stat

  // No rows at all: say so plainly rather than rendering a misleading zero.
  if (!stat || stat.total_requests === 0) {
    return (
      <span className='text-muted-foreground text-sm'>
        {t('No recorded usage')}
      </span>
    )
  }

  const hasUnknown = stat.unknown_tokens > 0
  const distinctLabel = t('{{count}} IP', {
    count: formatNumber(stat.total_ips, locale),
  })
  const headline = `${distinctLabel} · ${formatCompactNumber(stat.total_tokens, locale)} ${t('Tokens')}`

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant='ghost'
            aria-label={t('Caller IP usage: {{summary}}', {
              summary: headline,
            })}
            className='h-auto w-full min-w-0 justify-start px-0 py-0.5 text-left font-normal hover:bg-transparent'
          />
        }
      >
        <span className='flex min-w-0 flex-col items-start gap-0.5'>
          <span className='truncate text-sm font-medium'>{headline}</span>
          {hasUnknown && (
            <span className='text-muted-foreground truncate text-xs'>
              {t('{{count}} tokens from unrecorded IPs', {
                count: formatCompactNumber(stat.unknown_tokens, locale),
              })}
            </span>
          )}
        </span>
      </PopoverTrigger>
      <PopoverContent
        align='start'
        className='w-96 max-w-[calc(100vw-2rem)] gap-3 p-3'
      >
        <PopoverTitle>{t('Token usage by caller IP')}</PopoverTitle>
        <div className='max-h-72 overflow-y-auto'>
          <table className='w-full text-sm tabular-nums'>
            <thead>
              <tr className='text-muted-foreground border-b text-left text-xs'>
                <th className='pb-1 font-medium'>{t('IP')}</th>
                <th className='pb-1 text-right font-medium'>{t('Tokens')}</th>
                <th className='pb-1 text-right font-medium'>{t('Requests')}</th>
                <th className='pb-1 text-right font-medium'>
                  {t('Last Seen')}
                </th>
              </tr>
            </thead>
            <tbody>
              {stat.ips.map((row) => (
                <tr
                  key={row.ip || 'unknown'}
                  className='border-b last:border-0'
                >
                  <td
                    className={cn(
                      'max-w-40 truncate py-1 pr-2 font-mono text-xs',
                      !row.ip && 'text-muted-foreground italic'
                    )}
                  >
                    {row.ip || t('Not recorded')}
                  </td>
                  <td className='py-1 text-right'>
                    {formatNumber(
                      row.prompt_tokens + row.completion_tokens,
                      locale
                    )}
                  </td>
                  <td className='text-muted-foreground py-1 text-right'>
                    {formatNumber(row.requests, locale)}
                  </td>
                  <td className='text-muted-foreground py-1 text-right text-xs'>
                    {formatTimestampToDate(row.last_seen)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {stat.total_ips > stat.max_ips_per_token
            ? t(
                'Showing the top {{shown}} of {{total}} IP addresses by tokens consumed.',
                {
                  shown: stat.max_ips_per_token,
                  total: formatNumber(stat.total_ips, locale),
                }
              )
            : t('All {{total}} recorded IP addresses are listed.', {
                total: formatNumber(stat.total_ips, locale),
              })}
        </p>
        {hasUnknown && (
          <p className='text-muted-foreground text-xs leading-relaxed'>
            {t(
              'Usage recorded before IP logging was enabled has no address and cannot be attributed. It is counted above but cannot be traced to a caller.'
            )}
          </p>
        )}
      </PopoverContent>
    </Popover>
  )
}
