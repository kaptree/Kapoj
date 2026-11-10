import { flushPromises, shallowMount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia, type Pinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import type { Me } from '@/api/types'
import { useAuthStore } from '@/stores/auth'

import App from './App.vue'

const authApi = vi.hoisted(() => ({
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
}))

const usersApi = vi.hoisted(() => ({
  getMe: vi.fn(),
  updateMe: vi.fn(),
}))

const siteApi = vi.hoisted(() => ({
  getSite: vi.fn(),
  recordSiteVisit: vi.fn(),
}))

const leaderboardApi = vi.hoisted(() => ({
  signInActivity: vi.fn(),
}))

const toast = vi.hoisted(() => ({
  add: vi.fn(),
}))

vi.mock('@/api/auth', () => authApi)
vi.mock('@/api/users', () => usersApi)
vi.mock('@/api/site', () => siteApi)
vi.mock('@/api/leaderboard', () => leaderboardApi)
vi.mock('primevue/usetoast', () => ({
  useToast: () => toast,
}))

const me: Me = {
  id: 1,
  username: 'alice',
  nickname: 'Alice',
  email: 'alice@example.com',
  avatarUrl: '',
  avatarUploaded: false,
  role: 'user',
  roles: [],
  permissions: [],
  isSuperAdmin: false,
  rating: 1500,
  coins: 0,
  bio: '',
  status: 'active',
  createdAt: '2026-11-04T00:00:00Z',
}

const weekday = { awarded: false, activityPoints: 0, coins: 0, date: '2026-11-10' }

const routes = [{ path: '/', name: 'home', component: { template: '<div>home</div>' } }]

let wrapper: VueWrapper | undefined

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
  authApi.login.mockReset()
  authApi.register.mockReset()
  authApi.logout.mockReset()
  usersApi.getMe.mockReset()
  usersApi.updateMe.mockReset()
  siteApi.getSite.mockReset()
  siteApi.recordSiteVisit.mockReset()
  leaderboardApi.signInActivity.mockReset()
  toast.add.mockReset()

  usersApi.getMe.mockResolvedValue(me)
  siteApi.getSite.mockResolvedValue({})
  leaderboardApi.signInActivity.mockResolvedValue(weekday)
})

interface MountOptions {
  user?: Me | null
}

async function mountApp(options: MountOptions = {}) {
  const pinia: Pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  if (options.user) {
    auth.setSession({
      accessToken: 'access',
      refreshToken: 'refresh',
      sessionId: 'session-1',
      user: options.user,
    })
  }
  const fetchMe = vi.spyOn(auth, 'fetchMe').mockResolvedValue(undefined)

  const router: Router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/')
  await router.isReady()

  wrapper = shallowMount(App, {
    global: {
      plugins: [pinia, router],
      stubs: { RouterView: true },
    },
  })
  await flushPromises()

  return { auth, fetchMe, router }
}

describe('App auto sign-in', () => {
  it('signs in on session start and shows a success toast', async () => {
    leaderboardApi.signInActivity.mockResolvedValue({
      awarded: true,
      activityPoints: 5,
      coins: 5,
      date: '2026-11-10',
    })

    const { fetchMe } = await mountApp({ user: me })

    expect(leaderboardApi.signInActivity).toHaveBeenCalledTimes(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: '签到成功',
        detail: '活跃度 +5 · 金币 +5',
      }),
    )
    // Once for the startup refresh, once after a successful sign-in.
    expect(fetchMe).toHaveBeenCalledTimes(2)
  })

  it('does not toast when already signed in', async () => {
    leaderboardApi.signInActivity.mockResolvedValue({
      awarded: false,
      activityPoints: 0,
      coins: 0,
      date: '2026-11-10',
    })

    await mountApp({ user: me })

    expect(leaderboardApi.signInActivity).toHaveBeenCalledTimes(1)
    expect(toast.add).not.toHaveBeenCalled()
  })

  it('does not sign in for guests', async () => {
    await mountApp()

    expect(leaderboardApi.signInActivity).not.toHaveBeenCalled()
    expect(toast.add).not.toHaveBeenCalled()
  })

  it('silently ignores sign-in failure', async () => {
    leaderboardApi.signInActivity.mockRejectedValue(new ApiError(500, 50000, '服务器错误'))

    const { fetchMe } = await mountApp({ user: me })

    expect(leaderboardApi.signInActivity).toHaveBeenCalledTimes(1)
    expect(toast.add).not.toHaveBeenCalled()
    // Only the startup refresh: a failed sign-in must not trigger another.
    expect(fetchMe).toHaveBeenCalledTimes(1)
  })

  it('signs in after a login transition', async () => {
    const { auth } = await mountApp()

    expect(leaderboardApi.signInActivity).not.toHaveBeenCalled()

    leaderboardApi.signInActivity.mockResolvedValue({
      awarded: true,
      activityPoints: 5,
      coins: 5,
      date: '2026-11-10',
    })
    auth.setSession({
      accessToken: 'access',
      refreshToken: 'refresh',
      sessionId: 'session-1',
      user: me,
    })
    await flushPromises()

    expect(leaderboardApi.signInActivity).toHaveBeenCalledTimes(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '签到成功' }),
    )
  })

  it('tries once per user per local day', async () => {
    const { auth } = await mountApp({ user: me })

    expect(leaderboardApi.signInActivity).toHaveBeenCalledTimes(1)

    auth.setSession({
      accessToken: 'access-2',
      refreshToken: 'refresh-2',
      sessionId: 'session-2',
      user: { ...me, id: 2, username: 'bob', nickname: 'Bob' },
    })
    await flushPromises()

    expect(leaderboardApi.signInActivity).toHaveBeenCalledTimes(2)

    auth.setSession({
      accessToken: 'access',
      refreshToken: 'refresh',
      sessionId: 'session-1',
      user: me,
    })
    await flushPromises()

    expect(leaderboardApi.signInActivity).toHaveBeenCalledTimes(2)
  })
})
