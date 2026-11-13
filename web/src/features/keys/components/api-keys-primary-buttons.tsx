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
import { ArrowRightLeft, Link2, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useApiInfo } from '@/features/dashboard/hooks/use-status-data'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useApiKeys } from './api-keys-provider'

export function ApiKeysPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen } = useApiKeys()
  const isAdmin = useAuthStore(
    (state) => (state.auth.user?.role ?? 0) >= ROLE.ADMIN
  )
  // Creating a key is only the first half of the job; the next thing a user
  // needs is the base URL to call. Put the configured routes next to the button
  // that hands out the key, each with its own copy control.
  const { items: apiItems } = useApiInfo()
  return (
    <div className='flex gap-2'>
      {isAdmin && (
        <Button
          size='sm'
          variant='outline'
          onClick={() => setOpen('migrate-group')}
        >
          <ArrowRightLeft className='h-4 w-4' />
          {t('Migrate token groups')}
        </Button>
      )}
      {apiItems.length > 0 && (
        <Popover>
          <PopoverTrigger
            render={<Button size='sm' variant='outline' />}
            aria-label={t('API Base URL')}
          >
            <Link2 className='h-4 w-4' />
            {t('API Base URL')}
          </PopoverTrigger>
          <PopoverContent align='end' className='w-80 p-1'>
            <div className='flex flex-col'>
              {apiItems.map((item) => (
                <div
                  key={item.url}
                  className='hover:bg-muted/50 flex items-center gap-2 rounded-md px-2 py-1.5'
                >
                  <div className='min-w-0 flex-1'>
                    <div className='text-xs font-medium'>{item.route}</div>
                    <div className='text-muted-foreground truncate font-mono text-xs'>
                      {item.url}
                    </div>
                  </div>
                  <CopyButton
                    value={item.url}
                    size='icon'
                    tooltip={t('Copy URL')}
                    aria-label={t('Copy URL')}
                  />
                </div>
              ))}
            </div>
          </PopoverContent>
        </Popover>
      )}
      <Button size='sm' onClick={() => setOpen('create')}>
        <Plus className='h-4 w-4' />
        {t('Create API Key')}
      </Button>
    </div>
  )
}
