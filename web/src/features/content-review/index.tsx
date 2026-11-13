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
import { Download, RefreshCw, Trash2 } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  contentReviewDownloadUrl,
  contentReviewExportUrl,
  cleanupContentReview,
  deleteContentReview,
  getContentReviewDetail,
  getContentReviewList,
  getContentReviewStatus,
} from './api'

function formatTime(ts: number): string {
  return formatTimestamp(ts)
}

function PreviewBlock(props: { label: string; text: string }) {
  const { t } = useTranslation()
  return (
    <div className='min-w-0 space-y-1'>
      <Label className='text-xs font-semibold'>{t(props.label)}</Label>
      <pre className='bg-muted/40 max-h-72 overflow-y-auto rounded-md border p-2 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap'>
        {props.text || '—'}
      </pre>
    </div>
  )
}

function DetailDialog(props: {
  id: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [confirmDelete, setConfirmDelete] = useState(false)

  const detailQuery = useQuery({
    queryKey: ['content-review-detail', props.id],
    queryFn: async () =>
      requireServerSuccess(await getContentReviewDetail(props.id!)),
    enabled: props.open && props.id != null,
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteContentReview(props.id!),
    onSuccess: () => {
      toast.success(t('Deleted'))
      queryClient.invalidateQueries({ queryKey: ['content-review'] })
      setConfirmDelete(false)
      props.onOpenChange(false)
      props.onDeleted()
    },
    onError: (error) => handleServerError(error),
  })

  const detail = detailQuery.data

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        <span className='flex items-center gap-2'>
          {t('Content Review')}
          {detail?.truncated && (
            <Badge variant='secondary' className='text-xs'>
              {t('Truncated')}
            </Badge>
          )}
        </span>
      }
      description={t(
        'View the captured request and AI reply for manual review'
      )}
      contentClassName='sm:max-w-3xl'
      contentHeight='min(80dvh, 760px)'
      bodyClassName='pr-1'
    >
      <div className='flex h-full min-h-0 flex-col gap-3'>
        <div className='text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs'>
          {detail && (
            <>
              <span>
                {t('ID')}: <span className='font-mono'>{detail.id}</span>
              </span>
              <span>
                {t('Request ID')}:{' '}
                <span className='font-mono'>{detail.request_id}</span>
              </span>
              <span>
                {t('User')} ID:{' '}
                <span className='font-mono'>{detail.user_id}</span>
              </span>
              <span>{formatTime(detail.created_at)}</span>
              {detail.model_name && <span>{detail.model_name}</span>}
              {detail.token_name && (
                <span>
                  {t('Token')}: {detail.token_name}
                </span>
              )}
            </>
          )}
        </div>
        {detailQuery.isLoading ? (
          <p className='text-muted-foreground flex-1 pt-8 text-center text-sm'>
            {t('Loading...')}
          </p>
        ) : detailQuery.isError ? (
          <p className='text-destructive flex-1 pt-8 text-center text-sm'>
            {t('Failed to load')}
          </p>
        ) : detail ? (
          <>
            <ScrollArea className='min-h-0 flex-1'>
              <div className='space-y-3 pr-3'>
                <PreviewBlock
                  label='Request (prompt)'
                  text={detail.request_text}
                />
                <PreviewBlock
                  label='Response (AI reply)'
                  text={detail.response_text}
                />
              </div>
            </ScrollArea>
            <div className='flex flex-wrap items-center justify-between gap-2 border-t pt-3'>
              <Button
                variant='outline'
                size='sm'
                render={<a href={contentReviewDownloadUrl(detail.id)} />}
              >
                <Download className='size-4' />
                {t('Download')}
              </Button>
              <Button
                variant='destructive'
                size='sm'
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className='size-4' />
                {t('Delete')}
              </Button>
            </div>
          </>
        ) : null}
      </div>
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={t('Delete this captured exchange?')}
        desc={t('This cannot be undone.')}
        destructive
        handleConfirm={() => deleteMutation.mutate()}
        isLoading={deleteMutation.isPending}
        confirmText={t('Delete')}
      />
    </Dialog>
  )
}

export function ContentReview() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [username, setUsername] = useState('')
  const [appliedUsername, setAppliedUsername] = useState('')
  const [detailId, setDetailId] = useState<number | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [confirmCleanup, setConfirmCleanup] = useState(false)

  const statusQuery = useQuery({
    queryKey: ['content-review-status'],
    queryFn: async () => requireServerSuccess(await getContentReviewStatus()),
    staleTime: 60_000,
  })

  const listQuery = useQuery({
    queryKey: ['content-review', page, appliedUsername],
    queryFn: async () => {
      const res = requireServerSuccess(
        await getContentReviewList({
          p: page,
          page_size: 20,
          username: appliedUsername || undefined,
        })
      )
      if (!res.success || res.data == null) {
        throw new Error(res.message || 'Request failed')
      }
      return res.data
    },
  })

  const cleanupMutation = useMutation({
    mutationFn: () => cleanupContentReview(),
    onSuccess: (result) => {
      toast.success(t('Cleaned {{count}} records', { count: result.deleted }))
      queryClient.invalidateQueries({ queryKey: ['content-review'] })
      setConfirmCleanup(false)
    },
    onError: (error) => handleServerError(error),
  })

  const rows = listQuery.data?.items ?? []
  const total = listQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / 20))

  const onSearch = useCallback(() => {
    setPage(1)
    setAppliedUsername(username.trim())
  }, [username])

  const exportHref = useMemo(
    () => contentReviewExportUrl({ username: appliedUsername || undefined }),
    [appliedUsername]
  )

  if (statusQuery.data && !statusQuery.data.enabled) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('Content Review')}</CardTitle>
          <CardDescription>
            {t(
              'Content capture is off. Turn it on in Settings → Operation settings → Content review to start recording request and reply bodies.'
            )}
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Content Review')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' size='sm' render={<a href={exportHref} />}>
          <Download className='size-4' />
          {t('Export')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={() => setConfirmCleanup(true)}
        >
          <RefreshCw className='size-4' />
          {t('Clean up')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <div className='flex flex-wrap items-end gap-2'>
            <div className='flex flex-col gap-1.5'>
              <Label htmlFor='content-review-username'>{t('Username')}</Label>
              <Input
                id='content-review-username'
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                placeholder={t('Filter by username')}
                className='w-56'
                onKeyDown={(event) => {
                  if (event.key === 'Enter') onSearch()
                }}
              />
            </div>
            <Button variant='outline' size='sm' onClick={onSearch}>
              {t('Search')}
            </Button>
          </div>

          <ConfirmDialog
            open={confirmCleanup}
            onOpenChange={setConfirmCleanup}
            title={t('Prune records past the retention window?')}
            desc={t(
              'Records older than the configured retention days will be deleted.'
            )}
            destructive
            handleConfirm={() => cleanupMutation.mutate()}
            isLoading={cleanupMutation.isPending}
            confirmText={t('Clean up')}
          />

          <Card>
            <CardContent className='p-0'>
              {listQuery.isLoading ? (
                <p className='text-muted-foreground p-8 text-center text-sm'>
                  {t('Loading...')}
                </p>
              ) : listQuery.isError ? (
                <p className='text-destructive p-8 text-center text-sm'>
                  {t('Failed to load')}
                </p>
              ) : rows.length === 0 ? (
                <p className='text-muted-foreground p-8 text-center text-sm'>
                  {t(
                    'No records yet. New requests are captured while the switch is on.'
                  )}
                </p>
              ) : (
                <div className='divide-y'>
                  {rows.map((row) => (
                    <button
                      key={row.id}
                      type='button'
                      className='hover:bg-muted/40 w-full cursor-pointer px-4 py-3 text-left'
                      onClick={() => {
                        setDetailId(row.id)
                        setDetailOpen(true)
                      }}
                    >
                      <div className='flex flex-wrap items-center gap-2 text-xs'>
                        <span className='font-mono'>#{row.id}</span>
                        <span className='text-muted-foreground'>
                          {formatTime(row.created_at)}
                        </span>
                        <span className='font-medium'>{row.model_name}</span>
                        <span className='text-muted-foreground'>
                          {t('User')} ID: {row.user_id}
                        </span>
                        {row.token_name && (
                          <span className='text-muted-foreground'>
                            {t('Token')}: {row.token_name}
                          </span>
                        )}
                        {row.truncated && (
                          <Badge variant='secondary' className='text-xs'>
                            {t('Truncated')}
                          </Badge>
                        )}
                      </div>
                      <p className='text-muted-foreground mt-1 line-clamp-1 text-sm'>
                        {row.request_preview || '—'}
                      </p>
                      <p className='mt-0.5 line-clamp-1 text-sm'>
                        {row.response_preview || '—'}
                      </p>
                    </button>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          {totalPages > 1 && (
            <div className='text-muted-foreground flex items-center justify-between gap-3 text-sm'>
              <span>
                {t('Page {{current}} of {{total}}', {
                  current: page,
                  total: totalPages,
                })}
              </span>
              <div className='flex gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={page <= 1}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  {t('Previous page')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={page >= totalPages}
                  onClick={() => setPage((current) => current + 1)}
                >
                  {t('Next page')}
                </Button>
              </div>
            </div>
          )}

          <DetailDialog
            id={detailId}
            open={detailOpen}
            onOpenChange={setDetailOpen}
            onDeleted={() => void listQuery.refetch()}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
