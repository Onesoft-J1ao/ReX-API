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

import { AnimateInView } from '@/components/animate-in-view'

import { useLandingModels } from '../../hooks/use-landing-models'
import type { RouteKind } from '../../lib/default-landing-models'
import { RouteChart } from '../route-chart'

const KIND_META: Record<
  RouteKind,
  { lane: string; title: string; dot: string; rail: string }
> = {
  priority: {
    lane: 'A',
    title: 'Priority',
    dot: 'bg-emerald-400',
    rail: 'from-emerald-400/70',
  },
  free: {
    lane: 'B',
    title: 'Free models',
    dot: 'bg-sky-400',
    rail: 'from-sky-400/70',
  },
  value: {
    lane: 'C',
    title: 'Value & aggregated',
    dot: 'bg-amber-400',
    rail: 'from-amber-400/70',
  },
}

export function ServiceRoutes() {
  const { t } = useTranslation()
  const config = useLandingModels()

  return (
    <section className='relative z-10 px-6 py-16 sm:px-10 md:py-24 lg:px-14'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView className='mb-12 max-w-2xl md:mb-16'>
          <h2 className='text-foreground font-serif text-4xl leading-[1.05] font-medium tracking-[-0.02em] md:text-5xl'>
            {t('The same model,')}
            <br />
            {t('more than one service expectation.')}
          </h2>
          <p className='text-muted-foreground mt-6 max-w-xl text-base leading-7'>
            {t(
              'Price, speed, and stability come from different supply and maintenance paths. Start with a group, then choose a model.'
            )}
          </p>
        </AnimateInView>

        <div className='grid gap-4 md:grid-cols-3'>
          {config.serviceRoutes.map((group, i) => {
            const meta = KIND_META[group.kind] ?? KIND_META.value
            return (
              <AnimateInView
                key={group.id}
                delay={i * 80}
                animation='fade-up'
                className='landing-card-motion border-border/60 bg-card/40 group relative overflow-hidden rounded-2xl border p-7 md:p-8'
              >
                {/* Lane rail: the route's own supply path, drawn once down
                    the card edge so the three groups read as parallel tracks
                    rather than three interchangeable boxes. */}
                <span
                  aria-hidden='true'
                  className={`absolute inset-y-0 start-0 w-px bg-gradient-to-b ${meta.rail} via-transparent to-transparent`}
                />
                <div className='mb-7 flex items-center justify-between'>
                  <span className='text-muted-foreground flex items-center gap-2.5 font-mono text-[10px] tracking-[0.22em] uppercase'>
                    <span className={`size-1.5 rounded-full ${meta.dot}`} />
                    {t(meta.title)}
                  </span>
                  <span className='text-muted-foreground/50 font-mono text-[10px] tracking-[0.22em] uppercase'>
                    {t('Lane')} {meta.lane}
                  </span>
                </div>

                <p className='text-muted-foreground mb-6 text-sm leading-6'>
                  {t(group.description)}
                </p>

                <div className='border-border/50 flex items-baseline gap-2 border-t pt-5 font-mono text-xs'>
                  <span className='text-muted-foreground/70'>group</span>
                  <span className='text-primary'>{group.group}</span>
                </div>

                <ul className='mt-5 space-y-1'>
                  {group.models.map((model) => (
                    <li
                      key={model}
                      className='text-foreground/90 flex items-center gap-3 py-1.5 font-mono text-[13px]'
                    >
                      <span
                        aria-hidden='true'
                        className={`h-px w-3 shrink-0 bg-gradient-to-r ${meta.rail} to-transparent`}
                      />
                      <span className='truncate'>{model}</span>
                    </li>
                  ))}
                </ul>
              </AnimateInView>
            )
          })}
        </div>

        <div className='mt-14 grid items-center gap-10 lg:grid-cols-[1fr_20rem] lg:gap-14'>
          <RouteChart />
          <AnimateInView animation='fade-up' className='max-w-sm'>
            <p className='text-muted-foreground text-sm leading-6'>
              {t(
                'Automatic failover is available only for Claude Code and Codex. Keep a backup group strategy for production workloads.'
              )}
            </p>
            <p className='text-muted-foreground/70 mt-4 text-sm leading-6'>
              {t(
                'GPT and Claude APIs on this site are not official interfaces.'
              )}
            </p>
          </AnimateInView>
        </div>
      </div>
    </section>
  )
}
