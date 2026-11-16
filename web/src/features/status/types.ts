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

export type HealthLevel = 'excellent' | 'good' | 'warning' | 'critical'

/**
 * A model the group serves but that neither a probe nor traffic reached inside
 * the window. It carries the same fields as an observed model so the card can
 * render it in place, with the marker that nothing is known about it yet.
 */
export type UnknownHealthLevel = 'unknown'

/**
 * One observed traffic bucket for a model. The gateway buckets passive traffic
 * hourly, so a 24h window yields up to 24 of these and the card renders them as
 * a per-model history strip.
 */
export interface StatusBucket {
  ts: number
  requests: number
  ok_pct: number
  latency_ms: number
  /** Upstream mirror bucket: the hour was measured but carries no request count. */
  mirrored?: boolean
}

export interface StatusModel {
  model: string
  group: string
  availability: number
  latency_ms: number
  level: HealthLevel | UnknownHealthLevel
  channel_id?: number
  ok: boolean
  error_code?: number
  error_msg?: string
  has_active: boolean
  passive_requests?: number
  passive_success?: number
  series?: StatusBucket[]
  /** False for a served model with no probe and no traffic in the window. */
  has_data?: boolean
}

export interface StatusGroup {
  group: string
  /**
   * Multiplier this group charges on top of the base price. Absent on gateways
   * older than this field, in which case the card hides the badge rather than
   * inventing a 1x it never measured.
   */
  group_ratio?: number
  availability: number
  avg_latency_ms: number
  total_probes: number
  ok_probes: number
  level: HealthLevel
  models: StatusModel[]
  /**
   * False when the group reported neither probes nor traffic. Its availability
   * is still 100, but that figure is an assumption rather than a measurement,
   * so the card renders it grey instead of green.
   */
  observed?: boolean
}

export interface StatusCheckData {
  announcement: string
  groups: StatusGroup[]
  /**
   * Hour the per-model `series` buckets are anchored to. Absent on gateways
   * older than this field; a strip falls back to its earliest bucket then.
   */
  window_start?: number
}

export interface StatusCheckResponse {
  success: boolean
  message?: string
  data: StatusCheckData
}
