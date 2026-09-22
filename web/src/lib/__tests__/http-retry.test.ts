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
import {
  AxiosError,
  type AxiosAdapter,
  type InternalAxiosRequestConfig,
} from 'axios'
import i18next from 'i18next'
import { toast } from 'sonner'
import { afterEach, beforeAll, beforeEach, expect, it, vi } from 'vitest'

import { api } from '../api'
import {
  GATEWAY_RETRY_MESSAGES,
  isGatewayRetryExhausted,
  markGatewayRetryExhausted,
} from '../gateway-retry'
import { createAppQueryClient } from '../query-client'

const originalAdapter = api.defaults.adapter
const NOTICE = 'origin timeout'

function gatewayError(config: InternalAxiosRequestConfig, status: number) {
  return new AxiosError(
    'Gateway error',
    'ERR_BAD_RESPONSE',
    config,
    undefined,
    {
      data: { message: NOTICE },
      status,
      statusText: 'Gateway error',
      headers: {},
      config,
    }
  )
}

function networkError(config: InternalAxiosRequestConfig) {
  return new AxiosError('Network Error', 'ERR_NETWORK', config)
}

function okResponse(config: InternalAxiosRequestConfig) {
  return {
    data: { success: true },
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }
}

/** Drain the scheduled backoff so a retry fires without real waiting. */
async function advanceRetries(...delays: number[]) {
  for (const delay of delays) {
    await vi.advanceTimersByTimeAsync(delay)
  }
}

beforeAll(() => {
  // The global test instance ships empty resources, so register the real strings
  // to assert on what the user actually reads, interpolation included.
  i18next.addResourceBundle(
    'en',
    'translation',
    {
      [GATEWAY_RETRY_MESSAGES.RETRYING]: GATEWAY_RETRY_MESSAGES.RETRYING,
      [GATEWAY_RETRY_MESSAGES.EXHAUSTED]: GATEWAY_RETRY_MESSAGES.EXHAUSTED,
      [GATEWAY_RETRY_MESSAGES.REFRESH]: GATEWAY_RETRY_MESSAGES.REFRESH,
    },
    true,
    true
  )
})

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  api.defaults.adapter = originalAdapter
  toast.dismiss()
})

it('replays a 524 and hides the failure from the caller once the retry succeeds', async () => {
  const adapter = vi
    .fn<AxiosAdapter>()
    .mockImplementationOnce(async (config) => {
      throw gatewayError(config, 524)
    })
    .mockImplementationOnce(async (config) => okResponse(config))
  api.defaults.adapter = adapter
  const loading = vi.spyOn(toast, 'loading')
  const dismiss = vi.spyOn(toast, 'dismiss')

  const request = api.get('/api/status', { disableDuplicate: true })
  await advanceRetries(800)

  await expect(request).resolves.toMatchObject({ status: 200 })
  expect(adapter).toHaveBeenCalledTimes(2)
  expect(loading).toHaveBeenCalledTimes(1)
  expect(loading.mock.calls[0][0]).toBe('Network fluctuation, retrying… (1/3)')
  expect(loading.mock.calls[0][1]).toMatchObject({ duration: Infinity })
  // The retry notice must go away again, otherwise the bar keeps spinning.
  expect(dismiss).toHaveBeenCalledWith(loading.mock.calls[0][1]?.id)
})

it('replays a dropped connection that produced no response at all', async () => {
  const adapter = vi
    .fn<AxiosAdapter>()
    .mockImplementationOnce(async (config) => {
      throw networkError(config)
    })
    .mockImplementationOnce(async (config) => okResponse(config))
  api.defaults.adapter = adapter

  const request = api.get('/api/status', { disableDuplicate: true })
  await advanceRetries(800)

  await expect(request).resolves.toMatchObject({ status: 200 })
  expect(adapter).toHaveBeenCalledTimes(2)
})

it.each([400, 401, 404, 422, 500])(
  'lets a %s business error reach the caller without a retry',
  async (status) => {
    const adapter = vi
      .fn<AxiosAdapter>()
      .mockImplementationOnce(async (config) => {
        throw gatewayError(config, status)
      })
    api.defaults.adapter = adapter
    const loading = vi.spyOn(toast, 'loading')

    await expect(
      api.get('/api/status', { disableDuplicate: true })
    ).rejects.toMatchObject({ response: { status } })
    expect(adapter).toHaveBeenCalledTimes(1)
    expect(loading).not.toHaveBeenCalled()
  }
)

it('does not replay a POST that timed out at the edge, because the write may have landed', async () => {
  const adapter = vi
    .fn<AxiosAdapter>()
    .mockImplementationOnce(async (config) => {
      throw gatewayError(config, 524)
    })
  api.defaults.adapter = adapter
  const loading = vi.spyOn(toast, 'loading')

  await expect(
    api.post('/api/token/batch', { ids: [1] })
  ).rejects.toMatchObject({ response: { status: 524 } })
  expect(adapter).toHaveBeenCalledTimes(1)
  expect(loading).not.toHaveBeenCalled()
})

it('gives up after three retries and offers a refresh that sticks around', async () => {
  const adapter = vi.fn<AxiosAdapter>().mockImplementation(async (config) => {
    throw gatewayError(config, 524)
  })
  api.defaults.adapter = adapter
  const loading = vi.spyOn(toast, 'loading')
  const errorToast = vi.spyOn(toast, 'error')

  const request = api.get('/api/status', { disableDuplicate: true })
  const outcome = request.then(
    () => null,
    (error: AxiosError) => error
  )
  await advanceRetries(800, 1600, 3200)

  const failure = await outcome
  expect(failure?.response?.status).toBe(524)
  // One initial attempt plus three retries; the fourth failure is final.
  expect(adapter).toHaveBeenCalledTimes(4)
  expect(loading.mock.calls.map((call) => call[0])).toEqual([
    'Network fluctuation, retrying… (1/3)',
    'Network fluctuation, retrying… (2/3)',
    'Network fluctuation, retrying… (3/3)',
  ])
  expect(errorToast).toHaveBeenCalledTimes(1)
  const [message, options] = errorToast.mock.calls[0]
  expect(message).toBe(
    'Network is unstable. Please refresh the page and try again.'
  )
  expect(options).toMatchObject({ duration: Infinity })
  expect(options?.action).toMatchObject({ label: 'Refresh' })
  // Tagged so the query layer does not start a second backoff over the same
  // failure and hold the UI for another full cycle.
  expect(isGatewayRetryExhausted(failure)).toBe(true)
})

it('leaves an error the transport layer never retried untagged', async () => {
  const adapter = vi.fn<AxiosAdapter>().mockImplementation(async (config) => {
    throw gatewayError(config, 400)
  })
  api.defaults.adapter = adapter

  const failure = await api
    .get('/api/status', { disableDuplicate: true })
    .then(
      () => null,
      (error: AxiosError) => error
    )

  expect(isGatewayRetryExhausted(failure)).toBe(false)
})

it('stops the query layer from retrying a failure the transport layer exhausted', () => {
  const client = createAppQueryClient()
  const retry = client.getDefaultOptions().queries?.retry
  expect(typeof retry).toBe('function')
  const shouldRetry = retry as (count: number, error: unknown) => boolean

  const exhausted = new AxiosError('Offline', 'ERR_NETWORK')
  markGatewayRetryExhausted(exhausted)
  expect(shouldRetry(0, exhausted)).toBe(false)
  // Without the tag the guard does not fire, so the flag is what suppresses it.
  expect(isGatewayRetryExhausted(new AxiosError('Offline', 'ERR_NETWORK'))).toBe(
    false
  )
})

it('honours a per-request retry budget instead of the default three', async () => {
  const adapter = vi.fn<AxiosAdapter>().mockImplementation(async (config) => {
    throw gatewayError(config, 503)
  })
  api.defaults.adapter = adapter

  const request = api.get('/api/status', {
    disableDuplicate: true,
    gatewayRetryAttempts: 1,
  })
  const outcome = request.then(
    () => null,
    (error: AxiosError) => error
  )
  await advanceRetries(800)

  expect((await outcome)?.response?.status).toBe(503)
  expect(adapter).toHaveBeenCalledTimes(2)
})

it('skips the retry entirely when the caller opts out', async () => {
  const adapter = vi
    .fn<AxiosAdapter>()
    .mockImplementationOnce(async (config) => {
      throw gatewayError(config, 524)
    })
  api.defaults.adapter = adapter
  const loading = vi.spyOn(toast, 'loading')

  await expect(
    api.get('/api/status', {
      disableDuplicate: true,
      disableGatewayRetry: true,
    })
  ).rejects.toMatchObject({ response: { status: 524 } })
  expect(adapter).toHaveBeenCalledTimes(1)
  expect(loading).not.toHaveBeenCalled()
})
