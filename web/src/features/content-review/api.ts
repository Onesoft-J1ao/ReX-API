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
import { api } from '@/lib/api'

export type ContentReviewStatus = {
  enabled: boolean
  retention_days: number
}

export type ContentReviewListItem = {
  id: number
  request_id: string
  user_id: number
  created_at: number
  model_name: string
  token_name: string
  truncated: boolean
  request_preview: string
  response_preview: string
}

export type ContentReviewListResponse = {
  success: boolean
  message?: string
  data: {
    items: ContentReviewListItem[]
    total: number
    page: number
    page_size: number
  }
}

export type ContentReviewDetail = {
  id: number
  request_id: string
  user_id: number
  created_at: number
  request_text: string
  response_text: string
  model_name: string
  token_name: string
  truncated: boolean
}

function buildQuery(params: Record<string, unknown>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    search.set(key, String(value))
  }
  return search.toString()
}

export async function getContentReviewStatus(): Promise<ContentReviewStatus> {
  const res = await api.get('/api/content-review/status')
  return res.data.data
}

export async function getContentReviewList(params: {
  p?: number
  page_size?: number
  username?: string
  start_timestamp?: number
  end_timestamp?: number
}): Promise<ContentReviewListResponse> {
  const res = await api.get(
    `/api/content-review/?${buildQuery(params as Record<string, unknown>)}`
  )
  return res.data
}

export async function getContentReviewDetail(id: number): Promise<ContentReviewDetail> {
  const res = await api.get(`/api/content-review/${id}`)
  return res.data.data
}

export async function deleteContentReview(id: number): Promise<void> {
  await api.delete(`/api/content-review/${id}`)
}

export async function cleanupContentReview(): Promise<{ deleted: number }> {
  const res = await api.post('/api/content-review/cleanup')
  return res.data.data
}

export function contentReviewDownloadUrl(id: number): string {
  return `/api/content-review/${id}/download`
}

export function contentReviewExportUrl(params: {
  username?: string
  start_timestamp?: number
  end_timestamp?: number
}): string {
  return `/api/content-review/export?${buildQuery(params as Record<string, unknown>)}`
}
