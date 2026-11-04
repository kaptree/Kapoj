import axios, { AxiosError, type AxiosAdapter, type AxiosRequestConfig } from 'axios'

import type { TokenPair } from './types'

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: number,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export interface AuthHooks {
  getAccessToken(): string | null
  getRefreshToken(): string | null
  setTokens(pair: TokenPair): void
  onAuthFailure(): void
}

export interface Http {
  get<T>(url: string, config?: AxiosRequestConfig): Promise<T>
  post<T>(url: string, body?: unknown, config?: AxiosRequestConfig): Promise<T>
  put<T>(url: string, body?: unknown, config?: AxiosRequestConfig): Promise<T>
}

interface Envelope<T> {
  code: number
  message: string
  data: T
}

function toApiError(err: unknown): ApiError {
  if (err instanceof AxiosError && err.response) {
    const body = err.response.data as Partial<Envelope<unknown>> | undefined
    return new ApiError(
      err.response.status,
      body?.code ?? -1,
      body?.message ?? `请求失败（HTTP ${err.response.status}）`,
    )
  }
  return new ApiError(0, -1, '网络连接失败，请检查网络后重试')
}

/**
 * Creates the API client. On a 401 from a non-auth endpoint it exchanges the
 * refresh token once — concurrent failures share the same refresh — and
 * retries the request; if refreshing fails, onAuthFailure is called.
 */
export function createHttp(hooks: AuthHooks, options: { adapter?: AxiosAdapter } = {}): Http {
  const instance = axios.create({ baseURL: '/api', adapter: options.adapter })
  let refreshing: Promise<boolean> | null = null

  async function refresh(): Promise<boolean> {
    const refreshToken = hooks.getRefreshToken()
    if (!refreshToken) return false
    try {
      const res = await instance.post<Envelope<TokenPair>>('/auth/refresh', { refreshToken })
      hooks.setTokens(res.data.data)
      return true
    } catch {
      return false
    }
  }

  async function send<T>(config: AxiosRequestConfig, retried = false): Promise<T> {
    const token = hooks.getAccessToken()
    const headers = { ...(config.headers ?? {}) } as Record<string, string>
    if (token) headers.Authorization = `Bearer ${token}`

    try {
      const res = await instance.request<Envelope<T>>({ ...config, headers })
      return res.data.data
    } catch (e) {
      const err = toApiError(e)
      const isAuthEndpoint = config.url?.startsWith('/auth/') ?? false
      if (err.status === 401 && !retried && !isAuthEndpoint) {
        refreshing ??= refresh().finally(() => {
          refreshing = null
        })
        if (await refreshing) return send<T>(config, true)
        hooks.onAuthFailure()
      }
      throw err
    }
  }

  return {
    get: (url, config) => send({ ...config, method: 'GET', url }),
    post: (url, body, config) => send({ ...config, method: 'POST', url, data: body }),
    put: (url, body, config) => send({ ...config, method: 'PUT', url, data: body }),
  }
}
