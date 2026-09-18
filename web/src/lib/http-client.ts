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
import axios, { type AxiosRequestConfig } from 'axios'
import { t } from 'i18next'
import { toast } from 'sonner'

import {
  applyAuthRotation,
  clearAuthentication,
  getFreshAuthHeaders,
  refreshAuthentication,
} from '@/lib/auth-session'
import {
  GATEWAY_RETRY_MESSAGES,
  MAX_GATEWAY_RETRY_ATTEMPTS,
  getGatewayRetryDelayMs,
  markGatewayRetryExhausted,
  resolveGatewayRetry,
} from '@/lib/gateway-retry'
import { handleServerError } from '@/lib/handle-server-error'
import {
  getServerErrorMessage,
  safeServerErrorMessage,
} from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

declare module 'axios' {
  export interface AxiosRequestConfig {
    skipBusinessError?: boolean
    skipErrorHandler?: boolean
    disableDuplicate?: boolean
    skipAuthRefresh?: boolean
    authRetry?: boolean
    acceptAuthRotation?: boolean
    singleUseAuthorization?: boolean
    /** Retries already spent on this request; set by the retry interceptor. */
    gatewayRetryAttempt?: number
    /** Per-request retry budget override. Pass 0 to disable retries. */
    gatewayRetryAttempts?: number
    /** Opt a POST/PATCH into retrying when the replay is known to be safe. */
    gatewayRetryUnsafeMethods?: boolean
    /** Disable automatic gateway retries for this request entirely. */
    disableGatewayRetry?: boolean
  }
}

export type ApiRequestConfig = AxiosRequestConfig

export const api = axios.create({
  baseURL: '',
  withCredentials: true,
  headers: {
    // no-store forbids storage; no-cache also revalidates any older cached response.
    'Cache-Control': 'no-cache, no-store',
  },
})

const inFlightGet = new Map<string, Promise<unknown>>()
const originalGet = api.get.bind(api)

api.get = ((url: string, config: ApiRequestConfig = {}) => {
  if (config.disableDuplicate) return originalGet(url, config)

  const params = config.params ? JSON.stringify(config.params) : '{}'
  const sessionSID = useAuthStore.getState().auth.session?.sid || 'anonymous'
  const key = `${sessionSID}:${url}?${params}`
  const existingRequest = inFlightGet.get(key)
  if (existingRequest) return existingRequest

  const request = originalGet(url, config).finally(() => {
    inFlightGet.delete(key)
  })
  inFlightGet.set(key, request)
  return request
}) as typeof api.get

function redirectToSignIn(): void {
  if (
    typeof window !== 'undefined' &&
    window.location.pathname !== '/sign-in'
  ) {
    window.location.replace('/sign-in')
  }
}

api.interceptors.response.use(
  (response) => {
    if (response.config.acceptAuthRotation && response.data?.success === true) {
      applyAuthRotation(response.data.data)
    }

    return response
  },
  async (error) => {
    const config = error?.config as ApiRequestConfig | undefined
    const skipErrorHandler = config?.skipErrorHandler
    const status = error?.response?.status

    if (status === 401) {
      if (config && !config.skipAuthRefresh && !config.authRetry) {
        config.authRetry = true
        const outcome = await refreshAuthentication()
        if (outcome.kind === 'authenticated') {
          const token = useAuthStore.getState().auth.accessToken
          if (token) {
            config.headers = {
              ...config.headers,
              Authorization: `Bearer ${token}`,
            }
          }
          return api.request(config)
        }

        if (outcome.kind === 'anonymous' || outcome.kind === 'out_of_sync') {
          if (!skipErrorHandler) {
            handleServerError({
              message: t('Session expired!'),
              [safeServerErrorMessage]: true,
              cause: error,
            })
          }
          redirectToSignIn()
        }
      } else if (config?.authRetry) {
        clearAuthentication(false)
        if (!skipErrorHandler) {
          handleServerError({
            message: t('Session expired!'),
            [safeServerErrorMessage]: true,
            cause: error,
          })
        }
        redirectToSignIn()
      } else if (!skipErrorHandler) {
        handleServerError({
          message: t('Session expired!'),
          [safeServerErrorMessage]: true,
          cause: error,
        })
      }
    }
    if (axios.isAxiosError(error)) error.message = getServerErrorMessage(error)
    throw error
  }
)

// Cloudflare answers 524 when the origin is still busy past its own timeout, and
// 502/503/504 come from a restarting or saturated edge; a transport failure with
// no response at all is the same class of transient problem. Replaying those
// keeps a transient blip from turning into a bare error page, while 4xx and
// unsafe methods still surface immediately.
const GATEWAY_RETRY_TOAST_ID = 'gateway-retry'
const GATEWAY_RETRY_EXHAUSTED_TOAST_ID = 'gateway-retry-exhausted'

function notifyGatewayRetry(retryNumber: number): void {
  toast.loading(
    t(GATEWAY_RETRY_MESSAGES.RETRYING, {
      current: retryNumber,
      total: MAX_GATEWAY_RETRY_ATTEMPTS,
    }),
    { id: GATEWAY_RETRY_TOAST_ID, duration: Infinity }
  )
}

function notifyGatewayRetryExhausted(): void {
  toast.dismiss(GATEWAY_RETRY_TOAST_ID)
  // Sticky on purpose: the retries are gone, so the only way forward is a fresh
  // document, and a toast that fades would leave the user staring at nothing.
  toast.error(t(GATEWAY_RETRY_MESSAGES.EXHAUSTED), {
    id: GATEWAY_RETRY_EXHAUSTED_TOAST_ID,
    duration: Infinity,
    action: {
      label: t(GATEWAY_RETRY_MESSAGES.REFRESH),
      onClick: () => window.location.reload(),
    },
  })
}

function clearGatewayRetryNotice(): void {
  toast.dismiss(GATEWAY_RETRY_TOAST_ID)
}

function waitForGatewayRetry(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

api.interceptors.response.use(
  (response) => {
    if (response.config.gatewayRetryAttempt) clearGatewayRetryNotice()
    return response
  },
  async (error) => {
    const config = error?.config as ApiRequestConfig | undefined
    if (
      !config ||
      config.disableGatewayRetry ||
      error?.code === 'ERR_CANCELED'
    ) {
      throw error
    }

    const attemptsMade = config.gatewayRetryAttempt ?? 0
    const decision = resolveGatewayRetry({
      status: error?.response?.status,
      networkFailure: error?.response == null,
      method: config.method,
      attemptsMade,
      maxAttempts: config.gatewayRetryAttempts,
      allowNonIdempotentMethod: config.gatewayRetryUnsafeMethods,
    })

    if (decision === 'retry') {
      const retryNumber = attemptsMade + 1
      config.gatewayRetryAttempt = retryNumber
      notifyGatewayRetry(retryNumber)
      await waitForGatewayRetry(getGatewayRetryDelayMs(retryNumber))
      return api.request(config)
    }

    if (decision === 'exhausted') {
      notifyGatewayRetryExhausted()
      // The retries are spent, so the query layer must not start its own round
      // over the same failure; the user has already been offered a refresh.
      markGatewayRetryExhausted(error)
    }
    throw error
  }
)

api.interceptors.request.use(async (config) => {
  if (config.singleUseAuthorization || config.headers.has('X-Security-Proof')) {
    // Refresh before spending a proof/flow, never by replaying its request.
    config.skipAuthRefresh = true
    try {
      const headers = await getFreshAuthHeaders()
      for (const [name, value] of Object.entries(headers)) {
        config.headers.set(name, value)
      }
    } catch (error) {
      throw axios.AxiosError.from(error, undefined, config)
    }
    return config
  }
  const accessToken = useAuthStore.getState().auth.accessToken
  if (accessToken) {
    config.headers.Authorization = `Bearer ${accessToken}`
  }
  return config
})
