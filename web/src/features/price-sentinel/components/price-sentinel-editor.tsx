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
import { KeyRound, Percent, TrendingDown, TrendingUp } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
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
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  applyPriceSentinelEdit,
  syncPriceSentinelPrices,
  validatePriceSentinelToken,
} from '../api'
import type { PriceChange, PriceSentinelEditSession } from '../types'

type LoadState =
  | { kind: 'loading' }
  | { kind: 'ready'; session: PriceSentinelEditSession }
  | { kind: 'error'; message: string }

function ChangeDirection(props: { percent: number }) {
  const { t } = useTranslation()
  if (props.percent === 0) return null
  const dearer = props.percent > 0
  const Icon = dearer ? TrendingUp : TrendingDown
  return (
    <Badge variant={dearer ? 'destructive' : 'secondary'} className='gap-1'>
      <Icon className='size-3' aria-hidden />
      {dearer
        ? t('Upstream is {{percent}}% higher', {
            percent: formatNumber(Math.abs(props.percent)),
          })
        : t('Upstream is {{percent}}% lower', {
            percent: formatNumber(Math.abs(props.percent)),
          })}
    </Badge>
  )
}

function ChangeRow(props: {
  change: PriceChange
  adminToken: string
  token: string
  onApplied: (change: PriceChange, value: number) => void
}) {
  const { t } = useTranslation()
  const suggested = props.change.theirs ?? props.change.ours ?? 0
  const [value, setValue] = useState(String(suggested))
  const [applying, setApplying] = useState(false)

  useEffect(() => {
    setValue(String(props.change.theirs ?? props.change.ours ?? 0))
  }, [props.change])

  const apply = async () => {
    const parsed = Number(value)
    if (!Number.isFinite(parsed) || parsed < 0) {
      toast.error(t('Enter a non-negative number'))
      return
    }
    setApplying(true)
    try {
      await applyPriceSentinelEdit({
        token: props.token,
        adminToken: props.adminToken,
        modelName: props.change.model_name,
        field: props.change.field,
        value: parsed,
      })
      toast.success(
        t('Updated {{model}} {{field}}', {
          model: props.change.model_name,
          field: props.change.field,
        })
      )
      props.onApplied(props.change, parsed)
    } catch (error) {
      toast.error(
        getServerErrorMessage(error, t('Failed to update the price'))
      )
    } finally {
      setApplying(false)
    }
  }

  return (
    <tr className='border-b last:border-b-0'>
      <td className='py-2 pe-3 align-top'>
        <div className='font-medium break-all'>{props.change.model_name}</div>
        {props.change.groups && props.change.groups.length > 0 && (
          <div className='text-muted-foreground mt-1 flex flex-wrap gap-1'>
            {props.change.groups.map((group) => (
              <Badge key={group} variant='outline'>
                {group}
              </Badge>
            ))}
          </div>
        )}
      </td>
      <td className='py-2 pe-3 align-top font-mono text-xs'>
        {props.change.field}
      </td>
      <td className='py-2 pe-3 text-end align-top font-mono tabular-nums'>
        {props.change.ours == null ? t('Not set') : formatNumber(props.change.ours)}
      </td>
      <td className='py-2 pe-3 text-end align-top font-mono tabular-nums'>
        {props.change.theirs == null
          ? t('Not set')
          : formatNumber(props.change.theirs)}
      </td>
      <td className='py-2 pe-3 align-top'>
        <ChangeDirection percent={props.change.percent} />
      </td>
      <td className='py-2 align-top'>
        <div className='flex items-center justify-end gap-2'>
          <Input
            className='w-28'
            inputMode='decimal'
            value={value}
            aria-label={t('New value for {{model}}', {
              model: props.change.model_name,
            })}
            onChange={(event) => setValue(event.target.value)}
          />
          <Button size='sm' onClick={apply} disabled={applying}>
            {applying ? t('Applying...') : t('Apply')}
          </Button>
        </div>
      </td>
    </tr>
  )
}

export function PriceSentinelEditor(props: { token: string }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [state, setState] = useState<LoadState>({ kind: 'loading' })
  const [adminToken, setAdminToken] = useState('')
  const [applied, setApplied] = useState<Set<string>>(new Set())
  const [margin, setMargin] = useState('')
  const [syncing, setSyncing] = useState(false)

  const load = useCallback(async () => {
    setState({ kind: 'loading' })
    try {
      const session = await validatePriceSentinelToken(props.token)
      setState({ kind: 'ready', session })
    } catch (error) {
      setState({
        kind: 'error',
        message: getServerErrorMessage(
          error,
          t('This link is invalid or has expired.')
        ),
      })
    }
  }, [props.token, t])

  useEffect(() => {
    void load()
  }, [load])

  const pending = useMemo(() => {
    if (state.kind !== 'ready') return []
    return state.session.changes.filter(
      (change) => !applied.has(`${change.model_name}|${change.field}`)
    )
  }, [state, applied])

  const sync = async () => {
    // An empty field means "use the margin configured on the server", which is
    // why it parses to undefined rather than to 0: zero is a meaningful value
    // (follow the upstream exactly) and must not be sent by accident.
    const trimmed = margin.trim()
    const parsed = trimmed === '' ? undefined : Number(trimmed)
    if (parsed !== undefined && (!Number.isFinite(parsed) || parsed < 0)) {
      toast.error(t('Enter a non-negative number'))
      return
    }
    setSyncing(true)
    try {
      const result = await syncPriceSentinelPrices({
        token: props.token,
        adminToken,
        marginPercent: parsed === undefined ? undefined : parsed / 100,
      })
      toast.success(
        t('Applied {{applied}} prices and skipped {{skipped}} unpriced models.', {
          applied: result.applied,
          skipped: result.skipped,
        })
      )
      if (state.kind === 'ready') {
        setApplied(
          new Set(
            state.session.changes.map(
              (change) => `${change.model_name}|${change.field}`
            )
          )
        )
      }
    } catch (error) {
      toast.error(getServerErrorMessage(error, t('Failed to sync the prices')))
    } finally {
      setSyncing(false)
    }
  }

  if (state.kind === 'loading') {
    return <LoadingState message={t('Checking this link...')} />
  }

  if (state.kind === 'error') {
    return (
      <ErrorState
        title={t('This link cannot be used')}
        description={state.message}
      />
    )
  }

  const { session } = state
  const expiresAt = session.expires_at
    ? new Date(session.expires_at * 1000).toLocaleString(locale)
    : ''

  return (
    <div className='flex flex-col gap-4'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Upstream price changes')}</CardTitle>
          <CardDescription>
            {t(
              'This report compares our prices with the mirrored upstream. It only reports: nothing is changed until you apply it below.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-4'>
          {session.report && (
            <dl className='grid grid-cols-2 gap-3 text-sm sm:grid-cols-4'>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('Checked')}
                </dt>
                <dd>
                  {new Date(session.report.checked_at * 1000).toLocaleString(
                    locale
                  )}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('Compared')}
                </dt>
                <dd className='font-mono tabular-nums'>
                  {formatNumber(session.report.compared, locale)}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('We are higher')}
                </dt>
                <dd className='font-mono tabular-nums'>
                  {formatNumber(session.report.higher_price, locale)}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('We are lower')}
                </dt>
                <dd className='font-mono tabular-nums'>
                  {formatNumber(session.report.lower_price, locale)}
                </dd>
              </div>
            </dl>
          )}

          {expiresAt && (
            <p className='text-muted-foreground text-xs'>
              {t('This link stops working at {{time}}.', { time: expiresAt })}
            </p>
          )}

          <div className='flex flex-col gap-2'>
            <Label htmlFor='price-sentinel-admin-token' className='text-sm'>
              <KeyRound className='me-1 inline size-3.5' aria-hidden />
              {t('Admin access token')}
            </Label>
            <Input
              id='price-sentinel-admin-token'
              type='password'
              autoComplete='off'
              value={adminToken}
              onChange={(event) => setAdminToken(event.target.value)}
              aria-describedby='price-sentinel-admin-token-help'
            />
            <p
              id='price-sentinel-admin-token-help'
              className='text-muted-foreground text-xs'
            >
              {t(
                'The link proves the request came from us; this token proves you are an administrator. Both are required to write a price.'
              )}
            </p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Sync the whole report')}</CardTitle>
          <CardDescription>
            {t(
              'Writes every price below in one step, setting each to the upstream value plus the margin. Applying rows one by one is only needed for the exceptions.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <div className='flex flex-wrap items-end gap-3'>
            <div className='flex flex-col gap-2'>
              <Label htmlFor='price-sentinel-margin' className='text-sm'>
                <Percent className='me-1 inline size-3.5' aria-hidden />
                {t('Gross margin (%)')}
              </Label>
              <Input
                id='price-sentinel-margin'
                className='w-32'
                inputMode='decimal'
                placeholder={t('Server default')}
                value={margin}
                onChange={(event) => setMargin(event.target.value)}
                aria-describedby='price-sentinel-margin-help'
              />
            </div>
            <Button onClick={sync} disabled={syncing || pending.length === 0}>
              {syncing ? t('Syncing...') : t('Sync all')}
            </Button>
          </div>
          <p
            id='price-sentinel-margin-help'
            className='text-muted-foreground text-xs'
          >
            {t(
              'A 20 margin charges 20% above the upstream price. Leave it empty to use the margin configured on the server. Models we never priced are skipped, because syncing one would start charging for a model we do not sell.'
            )}
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Models that differ')}</CardTitle>
          <CardDescription>
            {t('{{count}} of {{compared}} compared models differ.', {
              count: pending.length,
              compared: session.report?.compared ?? 0,
            })}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {pending.length === 0 ? (
            <EmptyState
              title={t('Nothing left to apply')}
              description={t(
                'Every model in this report has been handled, or the upstream matches us.'
              )}
            />
          ) : (
            <div className='overflow-x-auto'>
              <table className='w-full text-sm'>
                <thead>
                  <tr className='text-muted-foreground border-b text-xs'>
                    <th className='py-2 pe-3 text-start font-medium'>
                      {t('Model')}
                    </th>
                    <th className='py-2 pe-3 text-start font-medium'>
                      {t('Field')}
                    </th>
                    <th className='py-2 pe-3 text-end font-medium'>
                      {t('Ours')}
                    </th>
                    <th className='py-2 pe-3 text-end font-medium'>
                      {t('Upstream')}
                    </th>
                    <th className='py-2 pe-3 text-start font-medium'>
                      {t('Difference')}
                    </th>
                    <th className='py-2 text-end font-medium'>
                      {t('Set to')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {pending.map((change) => (
                    <ChangeRow
                      key={`${change.model_name}|${change.field}`}
                      change={change}
                      token={props.token}
                      adminToken={adminToken}
                      onApplied={(appliedChange) =>
                        setApplied((current) => {
                          const next = new Set(current)
                          next.add(
                            `${appliedChange.model_name}|${appliedChange.field}`
                          )
                          return next
                        })
                      }
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
