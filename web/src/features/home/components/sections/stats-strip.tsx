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

For commercial licensing, please contact support@quantumnous.com
*/
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'

import { useLandingModels } from '../../hooks/use-landing-models'

/**
 * Instrument readout, not a metric card row.
 *
 * The dark world speaks in console language: one hairline rail carrying
 * labelled counters separated by dim ticks, the way a router status line
 * reads out live state. Deliberately not four equal boxes with a big number
 * over a small label — that template belongs to no particular product.
 */
export function StatsStrip() {
  const { t } = useTranslation()
  const config = useLandingModels()

  const readings = useMemo(() => {
    const models = config.serviceRoutes.reduce(
      (sum, group) => sum + group.models.length,
      0
    )
    return [
      { label: t('Models routed'), value: String(models) },
      { label: t('Route groups'), value: String(config.serviceRoutes.length) },
      { label: t('Model families'), value: String(config.coverage.length) },
      { label: t('Endpoint'), value: 'OpenAI-compatible' },
    ]
  }, [config, t])

  const families = useMemo(
    () => config.coverage.map((row) => row.family),
    [config.coverage]
  )

  return (
    <section className='relative z-10 px-6 pb-14 sm:px-10 md:pb-20 lg:px-14'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView animation='fade-in'>
          <div className='landing-scanline border-border/60 bg-[var(--surface-sunken)] relative overflow-hidden rounded-xl border'>
            <div className='flex flex-wrap items-center gap-x-10 gap-y-5 px-6 py-5 sm:px-8'>
              <span className='flex items-center gap-2 font-mono text-[10px] tracking-[0.22em] text-emerald-400 uppercase'>
                <span className='relative flex size-1.5'>
                  <span className='absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-60' />
                  <span className='relative inline-flex size-1.5 rounded-full bg-emerald-400' />
                </span>
                {t('Live')}
              </span>

              {readings.map((reading, i) => (
                <div key={reading.label} className='flex items-baseline gap-2.5'>
                  {i > 0 && (
                    <span
                      aria-hidden='true'
                      className='bg-border/70 me-7 hidden h-4 w-px self-center sm:block'
                    />
                  )}
                  <span className='text-foreground font-mono text-xl leading-none font-medium tabular-nums'>
                    {reading.value}
                  </span>
                  <span className='text-muted-foreground font-mono text-[10px] tracking-[0.18em] uppercase'>
                    {reading.label}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </AnimateInView>

        {families.length > 0 && (
          <div
            aria-hidden='true'
            className='border-border/50 text-muted-foreground/70 mt-4 overflow-hidden rounded-xl border'
          >
            <div className='rt-ticker flex w-max items-center gap-8 py-2.5 font-mono text-[10px] tracking-[0.18em] uppercase'>
              {Array.from({ length: 3 }, (_, k) => (
                <span key={k} className='flex items-center gap-8'>
                  {families.map((family) => (
                    <span
                      key={`${k}-${family}`}
                      className='flex items-center gap-8 whitespace-nowrap'
                    >
                      {family}
                      <span className='text-border'>/</span>
                    </span>
                  ))}
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    </section>
  )
}
