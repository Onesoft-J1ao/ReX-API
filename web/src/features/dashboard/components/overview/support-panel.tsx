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
import { ExternalLink, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { useStatus } from '@/hooks/use-status'

import { useDashboardContentVisibility } from '../../hooks/use-status-data'
import { PanelWrapper } from '../ui/panel-wrapper'

/**
 * Support panel: the operator's QQ group join link.
 *
 * The link is the one the group's own join widget points at — it carries the
 * group key and an auth key, so it is rendered as-is and never rebuilt from a
 * group number. The server sends it only when an operator has configured one,
 * so the panel disappears rather than showing a dead link.
 */
export function SupportPanel() {
  const { t } = useTranslation()
  const { support } = useDashboardContentVisibility()
  const { status } = useStatus()

  const url = typeof status?.support_qq_url === 'string' ? status.support_qq_url : ''

  if (!support) {
    return null
  }

  return (
    <PanelWrapper
      title={
        <span className='flex items-center gap-2'>
          <IconBadge tone='info' size='sm'>
            <Users />
          </IconBadge>
          {t('Support group')}
        </span>
      }
      description={t('Get help and product updates in our QQ group')}
      contentClassName='p-0'
    >
      <a
        href={url}
        target='_blank'
        rel='noopener noreferrer'
        className='hover:bg-muted/40 flex items-center justify-between gap-3 px-4 py-3 transition-colors sm:px-5'
      >
        <span className='min-w-0'>
          <span className='block text-sm font-medium'>
            {t('Join the QQ group')}
          </span>
          <span className='text-muted-foreground block text-xs'>
            {t('Open the group card in QQ to join.')}
          </span>
        </span>
        <ExternalLink className='text-muted-foreground size-4 shrink-0' />
      </a>
    </PanelWrapper>
  )
}
