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
import { createFileRoute } from '@tanstack/react-router'
import { Activity } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { PerformanceOverview } from '@/features/dashboard/components/models/performance-overview'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'

const REFRESH_INTERVAL_MS = 60 * 1000

function PerformancePageContent(props: {
  isLoading: boolean
  isError: boolean
  modelCount: number
}) {
  const { t } = useTranslation()
  if (props.isError) {
    return <p className='text-destructive text-sm'>{t('Request failed')}</p>
  }
  if (props.isLoading) return <PerformanceOverview />
  if (props.modelCount === 0) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('No performance data available')}
      </p>
    )
  }
  return <PerformanceOverview />
}

function PerformancePage() {
  const { t } = useTranslation()
  const { data, isLoading, isError } = useQuery({
    queryKey: ['perf-metrics-summary', 24],
    queryFn: () => getPerfMetricsSummary(24),
    staleTime: REFRESH_INTERVAL_MS,
    retry: false,
  })
  // The component issues this query too, but the page needs the success flag to
  // tell "nothing is reporting yet" apart from "the request failed" — the
  // component renders neither state.
  const modelCount = data?.data.models?.length ?? 0

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Relay Performance')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='min-w-0 space-y-4'>
          <div className='text-muted-foreground flex items-center gap-2 text-sm'>
            <Activity className='size-4' />
            <span>
              {t(
                'Success rate, latency and throughput from real relay traffic, merged with the upstream report'
              )}
            </span>
          </div>
          <PerformancePageContent
            isLoading={isLoading}
            isError={isError}
            modelCount={modelCount}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

/**
 * The relay-traffic view of model performance: success rate, latency,
 * throughput and per-group TTFT, read from real traffic.
 *
 * It is the /api/perf-metrics/summary view, which merges the mirrored upstream
 * summary over our own numbers — so a model the upstream already samples is
 * reported even when we carried no traffic for it. The sibling /status route is
 * the other half: live probe results rather than traffic.
 *
 * The route used to render the probe view, which made the two pages identical
 * and left the summary unreachable.
 */
export const Route = createFileRoute('/_authenticated/status/perf')({
  component: PerformancePage,
})