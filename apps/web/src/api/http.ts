import axios, {
  AxiosError,
  type AxiosAdapter,
  type AxiosRequestConfig,
  type AxiosResponse,
} from 'axios'

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
  delete<T>(url: string, config?: AxiosRequestConfig): Promise<T>
  /** Posts multipart/form-data; the browser sets the boundary header itself. */
  postForm<T>(url: string, form: FormData, config?: AxiosRequestConfig): Promise<T>
  /**
   * Fetches a raw binary resource (Bearer + refresh on 401) without parsing the
   * `{code,message,data}` envelope. The caller owns the returned Blob's lifetime.
   */
  getBlob(url: string, config?: AxiosRequestConfig): Promise<Blob>
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
      // Another tab may have rotated the token first; use its new pair.
      const current = hooks.getRefreshToken()
      return current !== null && current !== refreshToken
    }
  }

  async function request(config: AxiosRequestConfig, retried = false): Promise<AxiosResponse> {
    const isAuthEndpoint = config.url?.startsWith('/auth/') ?? false
    const token = hooks.getAccessToken()
    const headers = { ...(config.headers ?? {}) } as Record<string, string>
    // Auth endpoints identify the caller by the request body, so a stale
    // access token is never sent to them.
    if (token && !isAuthEndpoint) headers.Authorization = `Bearer ${token}`

    try {
      return await instance.request({ ...config, headers })
    } catch (e) {
      const err = toApiError(e)
      if (err.status === 401 && !retried && !isAuthEndpoint) {
        refreshing ??= refresh().finally(() => {
          refreshing = null
        })
        if (await refreshing) return request(config, true)
        hooks.onAuthFailure()
      }
      throw err
    }
  }

  async function send<T>(config: AxiosRequestConfig): Promise<T> {
    const res = await request(config)
    return (res.data as Envelope<T>).data
  }

  async function sendBlob(config: AxiosRequestConfig): Promise<Blob> {
    const res = await request({ ...config, responseType: 'blob' })
    return res.data as Blob
  }

  return {
    get: (url, config) => send({ ...config, method: 'GET', url }),
    post: (url, body, config) => send({ ...config, method: 'POST', url, data: body }),
    put: (url, body, config) => send({ ...config, method: 'PUT', url, data: body }),
    delete: (url, config) => send({ ...config, method: 'DELETE', url }),
    postForm: (url, form, config) => send({ ...config, method: 'POST', url, data: form }),
    getBlob: (url, config) => sendBlob({ ...config, method: 'GET', url }),
  }
}
