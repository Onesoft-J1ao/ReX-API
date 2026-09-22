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
import { Check, Download, ExternalLink } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'

import { isRecommendedBuild } from '../lib/platform'
import type { ClientTarget, DownloadLink, ToolDownload } from '../types'
import { ChecksumDialog } from './checksum-dialog'

type DownloadRowProps = {
  link: DownloadLink
  target: ClientTarget
}

function DownloadRow(props: DownloadRowProps) {
  const { t } = useTranslation()
  const recommended = isRecommendedBuild(props.link, props.target)
  const label = `${t(props.link.platformKey)} ${t(props.link.packageKey)}`
  const actionLabel = t(props.link.actionKey ?? 'Download')

  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border p-3',
        recommended ? 'border-primary/40 bg-primary/5' : 'border-border'
      )}
    >
      <div className='min-w-0 flex-1'>
        <div className='flex flex-wrap items-center gap-2'>
          <span className='text-sm font-medium'>{t(props.link.platformKey)}</span>
          {recommended ? (
            <Badge variant='secondary'>
              <Check aria-hidden='true' />
              {t('Recommended')}
            </Badge>
          ) : null}
        </div>
        <p className='text-muted-foreground text-xs'>
          {t(props.link.packageKey)}
        </p>
      </div>

      <div className='flex shrink-0 items-center gap-1.5'>
        <CopyButton
          value={props.link.url}
          size='icon'
          tooltip={t('Copy download link')}
        />
        {props.link.mirrorUrl ? (
          <Button
            variant='ghost'
            size='sm'
            aria-label={`${t('Mirror')} — ${label}`}
            render={
              <a
                href={props.link.mirrorUrl}
                target='_blank'
                rel='noopener noreferrer'
              />
            }
          >
            <ExternalLink className='size-3.5' aria-hidden='true' />
            {t('Mirror')}
          </Button>
        ) : null}
        <Button
          variant={recommended ? 'default' : 'outline'}
          size='sm'
          aria-label={`${actionLabel} — ${label}`}
          render={<a href={props.link.url} />}
        >
          <Download className='size-3.5' aria-hidden='true' />
          {actionLabel}
        </Button>
      </div>
    </div>
  )
}

type ToolCardProps = {
  tool: ToolDownload
  target: ClientTarget
}

export function ToolCard(props: ToolCardProps) {
  const { t } = useTranslation()

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex flex-wrap items-center gap-2'>
          {t(props.tool.name)}
          {props.tool.version ? (
            <Badge variant='outline'>v{props.tool.version}</Badge>
          ) : null}
        </CardTitle>
        <CardDescription>{t(props.tool.descriptionKey)}</CardDescription>
        <CardAction>
          <div className='flex items-center gap-1'>
            {props.tool.checksumsUrl ? (
              <ChecksumDialog url={props.tool.checksumsUrl} />
            ) : null}
            <Button
              variant='ghost'
              size='sm'
              aria-label={`${t('Open homepage')} — ${t(props.tool.name)}`}
              render={
                <a
                  href={props.tool.homepage}
                  target='_blank'
                  rel='noopener noreferrer'
                />
              }
            >
              <ExternalLink className='size-3.5' aria-hidden='true' />
              {t('Open homepage')}
            </Button>
          </div>
        </CardAction>
      </CardHeader>

      <CardContent className='flex flex-col gap-2'>
        {props.tool.links.map((link) => (
          <DownloadRow key={link.id} link={link} target={props.target} />
        ))}

        {props.tool.versionNoteKey ? (
          <p className='text-muted-foreground text-xs'>
            {t(props.tool.versionNoteKey, { version: props.tool.version })}
          </p>
        ) : null}
      </CardContent>

      <CardFooter className='flex-wrap gap-2'>
        <span className='text-muted-foreground text-xs'>
          {t('Backup downloads')}
        </span>
        {props.tool.backupLinks.map((backup) => (
          <Button
            key={backup.url}
            variant='link'
            size='xs'
            render={
              <a
                href={backup.url}
                target='_blank'
                rel='noopener noreferrer'
              />
            }
          >
            {t(backup.labelKey)}
          </Button>
        ))}
      </CardFooter>
    </Card>
  )
}
