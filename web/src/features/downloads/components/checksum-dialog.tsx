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
import { ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { fetchClaudeChecksums } from '../lib/checksums'

type ChecksumDialogProps = {
  url: string
}

/**
 * Show published SHA-256 checksums inside the app.
 *
 * The live manifest is read on open; when the browser cannot read it (the
 * mirror sends no CORS header) the bundled snapshot is shown instead and the
 * dialog says so, rather than presenting stale values as current.
 */
export function ChecksumDialog(props: ChecksumDialogProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  const checksums = useQuery({
    queryKey: ['downloads', 'checksums', props.url],
    queryFn: () => fetchClaudeChecksums(),
    enabled: open,
    staleTime: 5 * 60 * 1000,
  })

  const snapshot = checksums.data
  const manifestText = snapshot
    ? snapshot.entries
        .map((entry) => `${entry.sha256}  ${entry.fileName}`)
        .join('\n')
    : ''

  return (
    <Dialog
      open={open}
      onOpenChange={setOpen}
      title={t('SHA-256 checksums')}
      description={t(
        'Verify each downloaded file against its published SHA-256 checksum.'
      )}
      contentClassName='sm:max-w-3xl'
      trigger={
        <Button variant='ghost' size='sm'>
          <ShieldCheck className='size-3.5' />
          {t('Checksums')}
        </Button>
      }
      footer={
        <Button variant='outline' size='sm' onClick={() => setOpen(false)}>
          {t('Close')}
        </Button>
      }
    >
      {checksums.isPending ? (
        <div className='text-muted-foreground flex items-center justify-center gap-2 py-8 text-sm'>
          <Spinner />
          {t('Loading checksums…')}
        </div>
      ) : null}

      {snapshot && !snapshot.live ? (
        <Alert>
          <AlertDescription>
            {t(
              'Live checksums are unavailable from this browser. Showing the snapshot from {{date}}.',
              { date: snapshot.snapshotDate }
            )}
          </AlertDescription>
        </Alert>
      ) : null}

      {snapshot ? (
        <div className='mt-3 flex flex-col gap-3'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('File')}</TableHead>
                <TableHead>{t('Checksum')}</TableHead>
                <TableHead className='w-10' />
              </TableRow>
            </TableHeader>
            <TableBody>
              {snapshot.entries.map((entry) => (
                <TableRow key={entry.fileName}>
                  <TableCell className='font-medium'>
                    {entry.fileName}
                  </TableCell>
                  <TableCell className='font-mono text-xs break-all'>
                    {entry.sha256}
                  </TableCell>
                  <TableCell>
                    <CopyButton
                      value={entry.sha256}
                      size='icon'
                      tooltip={t('Copy checksum')}
                    />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>

          <div className='flex justify-end'>
            <CopyButton
              value={manifestText}
              variant='outline'
              size='sm'
              tooltip={t('Copy all checksums')}
            >
              <span className='px-1 text-xs'>{t('Copy all')}</span>
            </CopyButton>
          </div>
        </div>
      ) : null}
    </Dialog>
  )
}
