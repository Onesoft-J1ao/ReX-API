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
import { ExternalLink, ShoppingCart, Ticket } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { TitledCard } from '@/components/ui/titled-card'

const REDEMPTION_SHOP_URL = 'https://www.kufaka.com/shop/5753SF33'

export function RedemptionShopCard() {
  const { t } = useTranslation()

  return (
    <TitledCard
      title={t('Buy a Redemption Code')}
      description={t(
        'Buy a code from our shop and redeem it on this page to top up your balance instantly.'
      )}
      icon={<Ticket className='h-4 w-4' />}
      iconTone='warning'
      disableHoverEffect
      className='bg-accent ring-primary/25'
      contentClassName='space-y-3'
    >
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex flex-wrap items-center gap-2'>
          <StatusBadge
            label={t('Instant delivery')}
            variant='success'
            copyable={false}
          />
          <StatusBadge
            label={t('No service fee')}
            variant='info'
            copyable={false}
          />
          <span className='text-muted-foreground text-xs'>
            {t('Opens in a new tab')}
          </span>
        </div>
        <Button
          role='link'
          size='lg'
          className='w-full gap-2 sm:w-auto'
          render={
            <a
              href={REDEMPTION_SHOP_URL}
              target='_blank'
              rel='noopener noreferrer'
            />
          }
        >
          <ShoppingCart aria-hidden='true' className='h-4 w-4' />
          {t('Go to Shop')}
          <ExternalLink aria-hidden='true' className='h-4 w-4' />
        </Button>
      </div>
    </TitledCard>
  )
}
