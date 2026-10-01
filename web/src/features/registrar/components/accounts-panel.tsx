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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, Eye, EyeOff, FileUp, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { MaskedValueDisplay } from '@/components/masked-value-display'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { createServerError } from '@/lib/server-error-message'

import {
  downloadRegistrarAccounts,
  importRegistrarAccounts,
  listRegistrarAccounts,
} from '../api'

const PAGE_SIZE = 50
const MASK = '********'

interface RegistrarAccountsPanelProps {
  providerId: string
}

/**
 * The archived account log for one provider. Credentials stay masked until the
 * operator asks to reveal them, which the server audits.
 */
export function RegistrarAccountsPanel(props: RegistrarAccountsPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [reveal, setReveal] = useState(false)
  const [page, setPage] = useState(1)
  const [importOpen, setImportOpen] = useState(false)
  const [importText, setImportText] = useState('')

  const accountsQuery = useQuery({
    queryKey: ['registrar', 'accounts', props.providerId, page, reveal],
    queryFn: async () =>
      listRegistrarAccounts({
        providerId: props.providerId,
        page,
        pageSize: PAGE_SIZE,
        reveal,
      }),
    retry: false,
  })

  const importMutation = useMutation({
    mutationFn: async () => importRegistrarAccounts(props.providerId, importText),
    onSuccess: (result) => {
      toast.success(
        t('Imported {{imported}} accounts, skipped {{skipped}}.', {
          imported: result.imported,
          skipped: result.skipped,
        })
      )
      setImportText('')
      setImportOpen(false)
      void queryClient.invalidateQueries({ queryKey: ['registrar'] })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Import failed.')
      )
    },
  })

  const exportMutation = useMutation({
    mutationFn: async () => downloadRegistrarAccounts(props.providerId),
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Export failed.')
      )
    },
  })

  const data = accountsQuery.data
  const totalPages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : 1

  return (
    <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
      <div className='flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5'>
        <div className='min-w-0'>
          <div className='flex items-center gap-2'>
            <h3 className='text-sm font-semibold'>{t('Account archive')}</h3>
            {data && <Badge variant='outline'>{data.total}</Badge>}
          </div>
          <p className='text-muted-foreground mt-0.5 text-xs'>
            {t(
              'Fingerprint is the account password. Revealing or exporting it is audited.'
            )}
          </p>
        </div>
        <div className='flex shrink-0 flex-wrap items-center gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => setReveal((value) => !value)}
          >
            {reveal ? (
              <EyeOff data-icon='inline-start' className='size-3.5' aria-hidden='true' />
            ) : (
              <Eye data-icon='inline-start' className='size-3.5' aria-hidden='true' />
            )}
            {reveal ? t('Hide credentials') : t('Reveal credentials')}
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={exportMutation.isPending}
            onClick={() => exportMutation.mutate()}
          >
            <Download data-icon='inline-start' className='size-3.5' aria-hidden='true' />
            {t('Export archive')}
          </Button>
          <Dialog open={importOpen} onOpenChange={setImportOpen}>
            <DialogTrigger
              render={
                <Button type='button' variant='outline' size='sm'>
                  <FileUp data-icon='inline-start' className='size-3.5' aria-hidden='true' />
                  {t('Import archive')}
                </Button>
              }
            />
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{t('Import account archive')}</DialogTitle>
                <DialogDescription>
                  {t(
                    'Paste lines of username|fingerprint|key. Accounts whose key is already archived are skipped.'
                  )}
                </DialogDescription>
              </DialogHeader>
              <Textarea
                value={importText}
                onChange={(event) => setImportText(event.target.value)}
                rows={10}
                spellCheck={false}
                className='font-mono text-xs'
                aria-label={t('Account archive contents')}
              />
              <DialogFooter>
                <Button
                  type='button'
                  disabled={!importText.trim() || importMutation.isPending}
                  onClick={() => importMutation.mutate()}
                >
                  {importMutation.isPending ? t('Importing...') : t('Import')}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            aria-label={t('Refresh')}
            disabled={accountsQuery.isFetching}
            onClick={() => void accountsQuery.refetch()}
          >
            <RefreshCw
              className={accountsQuery.isFetching ? 'size-3.5 animate-spin' : 'size-3.5'}
              aria-hidden='true'
            />
          </Button>
        </div>
      </div>

      {accountsQuery.isLoading && (
        <div className='space-y-2 p-4 sm:p-5'>
          <Skeleton className='h-9 w-full rounded-md' />
          <Skeleton className='h-9 w-full rounded-md' />
          <Skeleton className='h-9 w-full rounded-md' />
        </div>
      )}

      {!accountsQuery.isLoading && accountsQuery.isError && (
        <ErrorState
          title={t('We could not load the account archive.')}
          description={
            accountsQuery.error instanceof Error
              ? accountsQuery.error.message
              : createServerError(accountsQuery.error).message
          }
          onRetry={() => {
            void accountsQuery.refetch()
          }}
          className='min-h-[200px]'
        />
      )}

      {!accountsQuery.isLoading && !accountsQuery.isError && data && (
        <>
          {data.items.length === 0 ? (
            <div className='text-muted-foreground px-4 py-8 text-center text-sm'>
              {t('No accounts archived yet.')}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Username')}</TableHead>
                  <TableHead>{t('Fingerprint')}</TableHead>
                  <TableHead>{t('API key')}</TableHead>
                  <TableHead>{t('Created')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.items.map((account) => (
                  <TableRow key={`${account.username}-${account.created_at}`}>
                    <TableCell className='font-mono text-xs'>
                      {account.username}
                    </TableCell>
                    <TableCell>
                      {reveal ? (
                        <MaskedValueDisplay
                          label={t('Fingerprint')}
                          fullValue={account.fingerprint}
                          maskedValue={MASK}
                          copyTooltip={t('Copy fingerprint')}
                          copyAriaLabel={t('Copy fingerprint')}
                        />
                      ) : (
                        <span className='text-muted-foreground font-mono text-xs'>
                          {MASK}
                        </span>
                      )}
                    </TableCell>
                    <TableCell>
                      {reveal ? (
                        <MaskedValueDisplay
                          label={t('API key')}
                          fullValue={account.api_key}
                          maskedValue={MASK}
                          copyTooltip={t('Copy API key')}
                          copyAriaLabel={t('Copy API key')}
                        />
                      ) : (
                        <span className='text-muted-foreground font-mono text-xs'>
                          {MASK}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className='text-muted-foreground text-xs'>
                      {new Date(account.created_at * 1000).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}

          {totalPages > 1 && (
            <div className='flex items-center justify-between border-t px-4 py-3 sm:px-5'>
              <span className='text-muted-foreground text-xs'>
                {t('Page {{page}} of {{total}}', {
                  page,
                  total: totalPages,
                })}
              </span>
              <div className='flex items-center gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page <= 1}
                  onClick={() => setPage((value) => Math.max(1, value - 1))}
                >
                  {t('Previous')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page >= totalPages}
                  onClick={() => setPage((value) => value + 1)}
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </section>
  )
}
