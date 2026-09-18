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
import { expect, it } from 'vitest'

import {
  GATEWAY_RETRY_MAX_DELAY_MS,
  MAX_GATEWAY_RETRY_ATTEMPTS,
  getGatewayRetryDelayMs,
  isGatewayRetryableMethod,
  isGatewayRetryableStatus,
  resolveGatewayRetry,
} from '../gateway-retry'

it.each([502, 503, 504, 524])(
  'treats %s as a transient gateway failure',
  (status) => {
    expect(isGatewayRetryableStatus(status)).toBe(true)
  }
)

it.each([200, 400, 401, 403, 404, 422, 429, 500])(
  'treats %s as something other than a gateway failure',
  (status) => {
    expect(isGatewayRetryableStatus(status)).toBe(false)
  }
)

it('retries a transport failure that never reached an origin', () => {
  expect(
    resolveGatewayRetry({
      networkFailure: true,
      attemptsMade: 0,
      method: 'get',
    })
  ).toBe('retry')
})

it('retries a gateway status for an idempotent method', () => {
  expect(
    resolveGatewayRetry({
      status: 524,
      networkFailure: false,
      attemptsMade: 0,
      method: 'GET',
    })
  ).toBe('retry')
})

it.each(['post', 'patch'])(
  'never replays a %s even when the gateway timed out, because the write may have landed',
  (method) => {
    expect(
      resolveGatewayRetry({
        status: 524,
        networkFailure: false,
        attemptsMade: 0,
        method,
      })
    ).toBe('pass-through')
    expect(
      resolveGatewayRetry({
        networkFailure: true,
        attemptsMade: 0,
        method,
      })
    ).toBe('pass-through')
  }
)

it('replays a non-idempotent method only when the caller opts in', () => {
  expect(
    resolveGatewayRetry({
      status: 524,
      networkFailure: false,
      attemptsMade: 0,
      method: 'post',
      allowNonIdempotentMethod: true,
    })
  ).toBe('retry')
})

it('leaves business errors alone so 4xx still reaches the caller immediately', () => {
  for (const status of [400, 401, 403, 404, 422, 500]) {
    expect(
      resolveGatewayRetry({
        status,
        networkFailure: false,
        attemptsMade: 0,
        method: 'get',
      })
    ).toBe('pass-through')
  }
})

it('stops at three retries and then asks the user to refresh', () => {
  const decide = (attemptsMade: number) =>
    resolveGatewayRetry({
      status: 524,
      networkFailure: false,
      attemptsMade,
      method: 'get',
    })

  expect(MAX_GATEWAY_RETRY_ATTEMPTS).toBe(3)
  expect(decide(0)).toBe('retry')
  expect(decide(1)).toBe('retry')
  expect(decide(2)).toBe('retry')
  expect(decide(3)).toBe('exhausted')
})

it('reports exhaustion only for failures that could have been retried', () => {
  expect(
    resolveGatewayRetry({
      status: 400,
      networkFailure: false,
      attemptsMade: 3,
      method: 'get',
    })
  ).toBe('pass-through')
  expect(
    resolveGatewayRetry({
      status: 524,
      networkFailure: false,
      attemptsMade: 3,
      method: 'post',
    })
  ).toBe('pass-through')
})

it('honours a per-request retry budget, including switching retries off', () => {
  expect(
    resolveGatewayRetry({
      status: 524,
      networkFailure: false,
      attemptsMade: 0,
      method: 'get',
      maxAttempts: 0,
    })
  ).toBe('pass-through')
  expect(
    resolveGatewayRetry({
      status: 524,
      networkFailure: false,
      attemptsMade: 0,
      method: 'get',
      maxAttempts: 1,
    })
  ).toBe('retry')
  expect(
    resolveGatewayRetry({
      status: 524,
      networkFailure: false,
      attemptsMade: 1,
      method: 'get',
      maxAttempts: 1,
    })
  ).toBe('exhausted')
})

it('backs off exponentially and never waits longer than the cap', () => {
  expect(getGatewayRetryDelayMs(1)).toBe(800)
  expect(getGatewayRetryDelayMs(2)).toBe(1600)
  expect(getGatewayRetryDelayMs(3)).toBe(3200)
  expect(getGatewayRetryDelayMs(4)).toBe(GATEWAY_RETRY_MAX_DELAY_MS)
  expect(getGatewayRetryDelayMs(9)).toBe(GATEWAY_RETRY_MAX_DELAY_MS)
})

it('falls back to GET semantics when the method is absent', () => {
  expect(isGatewayRetryableMethod(undefined)).toBe(true)
  expect(isGatewayRetryableMethod('DELETE')).toBe(true)
  expect(isGatewayRetryableMethod('post')).toBe(false)
})
