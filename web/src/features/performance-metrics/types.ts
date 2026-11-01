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
export type PerformanceSummary = {
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
}

export type PerformanceSeriesPoint = {
  ts: number
  avg_ttft_ms: number
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
}

export type PerformanceGroup = {
  group: string
  avg_ttft_ms: number
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
  series: PerformanceSeriesPoint[]
}

export type PerformanceMetricsData = {
  success: boolean
  message?: string
  data: {
    summary?: PerformanceSummary | null
    series?: PerformanceSeriesPoint[]
    window_start?: number
    window_end?: number
    model_name: string
    series_schema?: string
    groups: PerformanceGroup[]
  }
}

export type SuccessRatePoint = { ts: number; success_rate: number }

export type LatencyPoint = { ts: number; latency_ms: number }

export type PerfModelSummary = {
  model_name: string
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
  recent_success_series?: SuccessRatePoint[]
  recent_latency_series?: LatencyPoint[]
  request_count?: number
}

/**
 * One model's numbers inside one group. Health is per (model, group): the same
 * model routed through two groups is two independent upstreams, so a card scoped
 * to a group must read this rather than the model-wide `models` rollup, which
 * blends every group together.
 */
export type PerfGroupModelSummary = {
  group: string
  model_name: string
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
  recent_success_series?: SuccessRatePoint[]
  recent_latency_series?: LatencyPoint[]
}

export type PerfSummaryAllData = {
  success: boolean
  message?: string
  data: {
    summary?: PerformanceSummary | null
    window_start?: number
    window_end?: number
    models: PerfModelSummary[]
    /** Per-group split of the same figures; absent on older gateways. */
    group_models?: PerfGroupModelSummary[]
  }
}

export type StatusGroup = {
  group: string
  availability: number
  avg_latency_ms: number
  avg_ttft_ms: number
  cache_hit_rate: number
  cache_sample_count: number
  cache_input_tokens: number
  request_count: number
  availability_24h: number[]
  history_24h: StatusHistoryPoint[]
}

export type StatusHistoryPoint = {
  ts: number
  avg_latency_ms: number
  avg_ttft_ms: number
  ttft_sample_count: number
  cache_hit_rate: number
  cache_sample_count: number
  cache_input_tokens: number
}

export type StatusCheckData = {
  success: boolean
  message?: string
  data: {
    groups: StatusGroup[]
  }
}
