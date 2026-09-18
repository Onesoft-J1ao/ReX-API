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
/**
 * Retry policy for gateway-level failures.
 *
 * Cloudflare answers 524 when the origin is still working past its timeout, and
 * 502/503/504 come from a restarting or saturated edge. A transport failure with
 * no response at all (DNS, reset, connect timeout) is the same class of transient
 * problem. None of them say anything about the request itself, so they are worth
 * replaying automatically instead of dropping the user onto a bare error.
 *
 * Kept pure — no axios, no toasts, no i18n runtime — so the policy (which
 * statuses retry, which methods are safe to replay, how long to wait, how many
 * attempts) is testable on its own.
 */

/** Edge/gateway statuses that mean "retry later", not "your request is wrong". */
export const GATEWAY_RETRY_STATUSES = [502, 503, 504, 524] as const

/** Retries a single request gets before the user is asked to refresh. */
export const MAX_GATEWAY_RETRY_ATTEMPTS = 3

/** Backoff before the first retry; doubles per attempt and is capped. */
export const GATEWAY_RETRY_BASE_DELAY_MS = 800
export const GATEWAY_RETRY_MAX_DELAY_MS = 4000

/**
 * Only methods whose replay cannot change the outcome beyond the request's own
 * effect. POST/PATCH are excluded on purpose: a 524 is returned *after* the
 * origin stopped answering, so the write may already have landed upstream and a
 * replay could double-charge or duplicate a record. Callers that know a POST is
 * safe to replay opt in with `gatewayRetryMethods` on the request config.
 */
const RETRYABLE_METHODS = new Set(['get', 'head', 'options', 'put', 'delete'])

export type GatewayRetryDecision = 'retry' | 'exhausted' | 'pass-through'

export type GatewayRetryInput = {
  /** Status of the failure, when the request got a response at all. */
  status?: number
  /** True when the request failed before any response (DNS, reset, timeout). */
  networkFailure: boolean
  /** Request method; treated as GET when omitted, matching axios. */
  method?: string
  /** Retries already performed for this request. */
  attemptsMade: number
  /** Per-request retry budget override. */
  maxAttempts?: number
  /** Per-request opt-in for methods that are not retryable by default. */
  allowNonIdempotentMethod?: boolean
}

export function isGatewayRetryableStatus(status: number | undefined): boolean {
  if (status === undefined) return false
  return (GATEWAY_RETRY_STATUSES as readonly number[]).includes(status)
}

export function isGatewayRetryableMethod(
  method: string | undefined,
  allowNonIdempotentMethod = false
): boolean {
  if (allowNonIdempotentMethod) return true
  return RETRYABLE_METHODS.has((method ?? 'get').toLowerCase())
}

/**
 * Decide what the client should do with one failed request:
 * - `retry`: a transient gateway/transport failure with budget left.
 * - `exhausted`: same class of failure, but the retry budget is spent, so the
 *   user needs to be told to refresh.
 * - `pass-through`: anything else (4xx business errors, unsupported method,
 *   retries disabled) surfaces unchanged.
 */
export function resolveGatewayRetry(
  input: GatewayRetryInput
): GatewayRetryDecision {
  const retryableKind =
    isGatewayRetryableMethod(input.method, input.allowNonIdempotentMethod) &&
    (input.networkFailure || isGatewayRetryableStatus(input.status))
  if (!retryableKind) return 'pass-through'

  const maxAttempts = input.maxAttempts ?? MAX_GATEWAY_RETRY_ATTEMPTS
  if (maxAttempts <= 0) return 'pass-through'
  if (input.attemptsMade >= maxAttempts) return 'exhausted'

  return 'retry'
}

/**
 * Deterministic exponential backoff (800ms, 1.6s, 3.2s …, capped at 4s).
 * `retryNumber` is 1-based: 1 schedules the wait before the first retry.
 */
export function getGatewayRetryDelayMs(retryNumber: number): number {
  const exponent = Math.max(0, retryNumber - 1)
  return Math.min(
    GATEWAY_RETRY_BASE_DELAY_MS * 2 ** exponent,
    GATEWAY_RETRY_MAX_DELAY_MS
  )
}

/**
 * User-facing copy, kept here so the retry wording lives next to the policy it
 * describes. Values are i18n keys; render them through `t()`.
 */
export const GATEWAY_RETRY_MESSAGES = {
  RETRYING: 'Network fluctuation, retrying… ({{current}}/{{total}})',
  EXHAUSTED: 'Network is unstable. Please refresh the page and try again.',
  REFRESH: 'Refresh',
} as const
