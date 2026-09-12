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

export function StatsStrip() {
  const { t } = useTranslation()
  const config = useLandingModels()

  const stats = useMemo(() => {
    const models = config.serviceRoutes.reduce(
      (sum, group) => sum + group.models.length,
      0
    )
    return [
      { value: String(models), label: t('Models routed') },
      { value: String(config.serviceRoutes.length), label: t('Route groups') },
      { value: String(config.coverage.length), label: t('Model families') },
      { value: '1', label: t('OpenAI-compatible endpoint') },
    ]
  }, [config, t])

  const families = useMemo(
    () => config.coverage.map((row) => row.family),
    [config.coverage]
  )

  return (
    <section className='relative z-10 px-6 pt-4 pb-14 sm:px-10 md:pb-20 lg:px-14'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView animation='fade-up'>
          <dl className='border-border/60 bg-card/40 grid grid-cols-2 gap-px overflow-hidden rounded-2xl border lg:grid-cols-4'>
            {stats.map((stat) => (
              <div key={stat.label} className='bg-background/60 px-7 py-6'>
                <dt className='text-muted-foreground order-2 mt-1 text-xs leading-5'>
                  {stat.label}
                </dt>
                <dd className='text-foreground order-1 font-serif text-4xl font-medium tracking-tight tabular-nums'>
                  {stat.value}
                </dd>
              </div>
            ))}
          </dl>
        </AnimateInView>

        {families.length > 0 && (
          <div
            aria-hidden='true'
            className='border-border/60 text-muted-foreground/70 mt-6 overflow-hidden rounded-2xl border'
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
