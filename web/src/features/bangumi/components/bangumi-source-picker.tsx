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
import { Check, Layers } from 'lucide-react'
import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Switch } from '@/components/ui/switch'

import { formatBytes } from '../lib/playback-state'
import type { BangumiRelease } from '../types'

export function BangumiSourcePicker(props: {
  releases: BangumiRelease[]
  selectedId?: string
  episodeNumber?: number
  onSelect: (release: BangumiRelease) => void
}) {
  const { t } = useTranslation()
  const allId = useId()
  const [query, setQuery] = useState('')
  const [showAll, setShowAll] = useState(false)
  const groups = useMemo(() => {
    const grouped = new Map<string, BangumiRelease[]>()
    const term = query.trim().toLowerCase()
    for (const release of props.releases) {
      if (
        !showAll &&
        props.episodeNumber != null &&
        release.episodeNumber !== 0 &&
        release.episodeNumber !== props.episodeNumber
      ) {
        continue
      }
      if (
        term &&
        !`${release.title} ${release.alliance}`.toLowerCase().includes(term)
      ) {
        continue
      }
      const name = release.alliance.trim()
      const items = grouped.get(name) ?? []
      items.push(release)
      grouped.set(name, items)
    }
    return [...grouped.entries()]
  }, [props.releases, props.episodeNumber, query, showAll])
  const selectedGroup = props.releases
    .find((release) => release.topicId === props.selectedId)
    ?.alliance.trim()
  const initialGroup =
    groups.find(([name]) => name === selectedGroup)?.[0] ?? groups[0]?.[0]

  return (
    <section aria-label={t('Sources')} className='min-w-0 space-y-3'>
      <div className='flex items-center justify-between gap-2'>
        <h3 className='text-sm font-semibold'>{t('Sources')}</h3>
        <span className='text-muted-foreground text-xs'>
          Mikan · BitTorrent
        </span>
      </div>
      <Input
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        aria-label={t('Filter releases')}
        placeholder={t('Filter releases')}
      />
      <div className='flex items-center gap-2'>
        <Switch
          id={allId}
          size='sm'
          checked={showAll}
          onCheckedChange={setShowAll}
        />
        <Label htmlFor={allId}>{t('Show all episodes')}</Label>
      </div>
      <ScrollArea className='max-h-96 lg:h-[28rem] lg:max-h-none'>
        {groups.length === 0 ? (
          <EmptyState
            className='min-h-0 py-6'
            title={t('No source matched this episode. Pick one below.')}
            action={
              !showAll && !query ? (
                <Button variant='outline' onClick={() => setShowAll(true)}>
                  {t('Show all episodes')}
                </Button>
              ) : undefined
            }
          />
        ) : (
          <Accordion
            key={`${showAll}:${query}:${selectedGroup ?? ''}`}
            multiple
            defaultValue={initialGroup == null ? [] : [initialGroup]}
          >
            {groups.map(([name, releases]) => (
              <AccordionItem key={name} value={name}>
                <AccordionTrigger className='min-h-9 items-center gap-2 hover:no-underline'>
                  <Layers
                    aria-hidden
                    className='text-muted-foreground size-4 shrink-0'
                  />
                  <span className='min-w-0 flex-1 truncate'>
                    {name || t('Unknown group')}
                  </span>
                  {releases.some(
                    (release) => release.topicId === props.selectedId
                  ) && (
                    <Check
                      aria-hidden
                      className='text-primary size-4 shrink-0'
                    />
                  )}
                  <span className='text-muted-foreground text-xs'>
                    {t('Releases: {{count}}', { count: releases.length })}
                  </span>
                </AccordionTrigger>
                <AccordionContent>
                  <div className='flex flex-wrap gap-2 px-1 pb-2'>
                    {releases.map((release) => (
                      <Button
                        key={release.topicId}
                        variant={
                          release.topicId === props.selectedId
                            ? 'secondary'
                            : 'outline'
                        }
                        aria-label={release.title}
                        aria-pressed={release.topicId === props.selectedId}
                        title={release.title}
                        className='h-auto min-h-9 max-w-full flex-col items-start gap-1 p-2 text-left whitespace-normal'
                        onClick={() => props.onSelect(release)}
                      >
                        <span className='text-sm'>
                          {[
                            release.resolution,
                            release.subtitleLanguages.join('/'),
                            release.videoCodec,
                            release.container,
                          ]
                            .filter(Boolean)
                            .join(' · ') || t('Unspecified format')}
                        </span>
                        <span className='text-muted-foreground flex flex-wrap gap-x-2 text-xs'>
                          {release.episodeNumber === 0 && (
                            <span>{t('Batch or unspecified episode')}</span>
                          )}
                          <span>{formatBytes(release.size)}</span>
                          {!release.playable && (
                            <span className='text-warning'>
                              {t('Browser compatibility is not verified')}
                            </span>
                          )}
                        </span>
                      </Button>
                    ))}
                  </div>
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        )}
      </ScrollArea>
    </section>
  )
}
