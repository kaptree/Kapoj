import { describe, expect, it, vi } from 'vitest'
import { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'

import { ApiError, createHttp, type AuthHooks } from './http'

type Handler = (config: InternalAxiosRequestConfig) => Promise<AxiosResponse>

function respond(config: InternalAxiosRequestConfig, status: number, body: unknown) {
  const response = { data: body, status, statusText: '', headers: {}, config } as AxiosResponse
  if (status >= 200 && status < 300) return Promise.resolve(response)
  return Promise.reject(new AxiosError('fail', String(status), config, null, response))
}

function hooks(overrides: Partial<AuthHooks> = {}): AuthHooks {
  let access: string | null = 'old-access'
  let refresh: string | null = 'old-refresh'
  return {
    getAccessToken: () => access,
    getRefreshToken: () => refresh,
    setTokens: (pair) => {
      access = pair.accessToken
      refresh = pair.refreshToken
    },
    onAuthFailure: vi.fn(),
    ...overrides,
  }
}

describe('createHttp', () => {
  it('unwraps data', async () => {
    const http = createHttp(hooks(), {
      adapter: (c) => respond(c, 200, { code: 0, message: 'ok', data: { id: 1 } }),
    })
    await expect(http.get('/users/me')).resolves.toEqual({ id: 1 })
  })

  it('sends the access token as a bearer header', async () => {
    let seen: unknown
    const http = createHttp(hooks(), {
      adapter: (c) => {
        seen = c.headers.Authorization
        return respond(c, 200, { code: 0, message: 'ok', data: null })
      },
    })
    await http.get('/users/me')
    expect(seen).toBe('Bearer old-access')
  })

  it('throws ApiError with code', async () => {
    const http = createHttp(hooks(), {
      adapter: (c) => respond(c, 409, { code: 40901, message: '用户名已被占用', data: null }),
    })
    const err = await http.post('/auth/register', {}).catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 409, code: 40901, message: '用户名已被占用' })
  })

  it('reports network failures as ApiError', async () => {
    const http = createHttp(hooks(), {
      adapter: (c) => Promise.reject(new AxiosError('Network Error', 'ERR_NETWORK', c)),
    })
    const err = (await http.get('/users/me').catch((e) => e)) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(0)
  })

  it('refreshes once and retries on 401', async () => {
    const h = hooks()
    const calls: string[] = []
    const http = createHttp(h, {
      adapter: (c) => {
        calls.push(`${c.url} ${c.headers.Authorization ?? ''}`.trim())
        if (c.url === '/auth/refresh') {
          return respond(c, 200, {
            code: 0,
            message: 'ok',
            data: { accessToken: 'new-access', refreshToken: 'new-refresh' },
          })
        }
        if (c.headers.Authorization === 'Bearer new-access') {
          return respond(c, 200, { code: 0, message: 'ok', data: 'fresh' })
        }
        return respond(c, 401, { code: 40100, message: 'expired', data: null })
      },
    })

    await expect(http.get('/users/me')).resolves.toBe('fresh')
    expect(calls).toEqual([
      '/users/me Bearer old-access',
      '/auth/refresh',
      '/users/me Bearer new-access',
    ])
    expect(h.getRefreshToken()).toBe('new-refresh')
  })

  it('concurrent 401s share one refresh', async () => {
    let refreshCalls = 0
    let releaseRefresh!: () => void
    const refreshGate = new Promise<void>((r) => (releaseRefresh = r))
    const handler: Handler = async (c) => {
      if (c.url === '/auth/refresh') {
        refreshCalls++
        await refreshGate
        return respond(c, 200, {
          code: 0,
          message: 'ok',
          data: { accessToken: 'new-access', refreshToken: 'new-refresh' },
        })
      }
      if (c.headers.Authorization === 'Bearer new-access') {
        return respond(c, 200, { code: 0, message: 'ok', data: c.url })
      }
      return respond(c, 401, { code: 40100, message: 'expired', data: null })
    }
    const http = createHttp(hooks(), { adapter: handler })

    const pending = Promise.all([http.get('/a'), http.get('/b'), http.get('/c')])
    await new Promise((r) => setTimeout(r, 0))
    releaseRefresh()

    await expect(pending).resolves.toEqual(['/a', '/b', '/c'])
    expect(refreshCalls).toBe(1)
  })

  it('refresh failure clears auth', async () => {
    const h = hooks()
    const http = createHttp(h, {
      adapter: (c) => respond(c, 401, { code: 40100, message: 'expired', data: null }),
    })
    const err = await http.get('/users/me').catch((e) => e)
    expect(err).toMatchObject({ status: 401, code: 40100 })
    expect(h.onAuthFailure).toHaveBeenCalledTimes(1)
  })

  it('does not try to refresh for auth endpoints', async () => {
    const h = hooks()
    const urls: string[] = []
    const http = createHttp(h, {
      adapter: (c) => {
        urls.push(c.url!)
        return respond(c, 401, { code: 40101, message: '账号或密码错误', data: null })
      },
    })
    await http.post('/auth/login', {}).catch(() => {})
    expect(urls).toEqual(['/auth/login'])
    expect(h.onAuthFailure).not.toHaveBeenCalled()
  })

  it('does not send the access token to auth endpoints', async () => {
    let seen: unknown = 'unset'
    const http = createHttp(hooks(), {
      adapter: (c) => {
        seen = c.headers.Authorization
        return respond(c, 200, { code: 0, message: 'ok', data: null })
      },
    })
    await http.post('/auth/logout', { refreshToken: 'r' })
    expect(seen).toBeUndefined()
  })

  it('adopts tokens rotated by another tab instead of logging out', async () => {
    const h = hooks()
    const http = createHttp(h, {
      adapter: (c) => {
        if (c.url === '/auth/refresh') {
          // Another tab already rotated the refresh token and stored the new pair.
          h.setTokens({ accessToken: 'tab-access', refreshToken: 'tab-refresh' })
          return respond(c, 401, { code: 40100, message: 'reused', data: null })
        }
        if (c.headers.Authorization === 'Bearer tab-access') {
          return respond(c, 200, { code: 0, message: 'ok', data: 'ok-from-tab' })
        }
        return respond(c, 401, { code: 40100, message: 'expired', data: null })
      },
    })

    await expect(http.get('/users/me')).resolves.toBe('ok-from-tab')
    expect(h.onAuthFailure).not.toHaveBeenCalled()
  })
})
