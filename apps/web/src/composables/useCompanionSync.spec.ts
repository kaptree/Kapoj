import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'

import type { CompanionEvent, CompanionState, Me, Session } from '@/api/types'
import { COMPANION_LINES } from '@/content/companion/lines.zh-CN'
import { useAuthStore } from '@/stores/auth'
import { useCompanionStore } from '@/stores/companion'
import {
  COMPANION_EVENT_POLL_MS,
  companionEventLineKey,
  useCompanionSync,
} from './useCompanionSync'

const companionApi = vi.hoisted(() => ({
  getCompanion: vi.fn(),
  listCompanionEvents: vi.fn(),
  markCompanionEventsRead: vi.fn(),
}))

vi.mock('@/api/companion', () => companionApi)

enableAutoUnmount(afterEach)

function remoteState(overrides: Partial<CompanionState> = {}): CompanionState {
  return {
    bond: 120,
    level: 3,
    levelName: '搭档',
    nextThreshold: 170,
    dailyGain: 4,
    dailyCap: 15,
    activityLevel: 'normal',
    dockEnabled: true,
    publicProfile: false,
    equippedSkin: '',
    unlockedSkins: [],
    unreadCount: 2,
    todayEvents: [],
    ...overrides,
  }
}

function companionEvent(id: number, type: string): CompanionEvent {
  return {
    id,
    type,
    bond: 1,
    payload: {},
    readAt: null,
    createdAt: '2026-11-17T08:00:00Z',
  }
}

let visibility: DocumentVisibilityState = 'visible'

function setVisibility(value: DocumentVisibilityState): void {
  visibility = value
}

function signIn(id = 7): void {
  useAuthStore().setSession({
    accessToken: `access-${id}`,
    refreshToken: `refresh-${id}`,
    sessionId: `session-${id}`,
    user: { id } as unknown as Me,
  } as Session)
}

function mountSync() {
  const Comp = defineComponent({
    setup() {
      useCompanionSync()
      return () => null
    },
  })
  return mount(Comp)
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
  visibility = 'visible'
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    get: () => visibility,
  })

  companionApi.getCompanion.mockReset()
  companionApi.listCompanionEvents.mockReset()
  companionApi.markCompanionEventsRead.mockReset()
  companionApi.getCompanion.mockResolvedValue(remoteState())
  companionApi.listCompanionEvents.mockResolvedValue([])
  companionApi.markCompanionEventsRead.mockResolvedValue(undefined)

  // Only the interval is faked so flushPromises() can still use setImmediate.
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  Reflect.deleteProperty(document, 'visibilityState')
})

describe('useCompanionSync', () => {
  it('fetches the remote state once a user is logged in', async () => {
    signIn(7)
    mountSync()
    await flushPromises()

    const store = useCompanionStore()
    expect(companionApi.getCompanion).toHaveBeenCalledTimes(1)
    expect(store.bond).toBe(120)
    expect(store.level).toBe(3)
    expect(store.levelName).toBe('搭档')
    expect(store.nextThreshold).toBe(170)
    expect(store.dailyGain).toBe(4)
    expect(store.dailyCap).toBe(15)
    expect(store.unreadCount).toBe(2)
  })

  it('makes no companion requests for guests', async () => {
    mountSync()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(COMPANION_EVENT_POLL_MS * 2)

    expect(companionApi.getCompanion).not.toHaveBeenCalled()
    expect(companionApi.listCompanionEvents).not.toHaveBeenCalled()
  })

  it('polls unread events every 60 seconds and marks them read', async () => {
    signIn()
    mountSync()
    await flushPromises()

    // The dock polls immediately on login, then on the interval.
    expect(companionApi.listCompanionEvents).toHaveBeenCalledTimes(1)

    companionApi.listCompanionEvents.mockResolvedValue([
      companionEvent(1, 'ac:daily'),
      companionEvent(2, 'signin'),
    ])

    await vi.advanceTimersByTimeAsync(COMPANION_EVENT_POLL_MS)
    await flushPromises()

    expect(companionApi.listCompanionEvents).toHaveBeenCalledTimes(2)
    expect(companionApi.markCompanionEventsRead).toHaveBeenCalledWith([1, 2])

    const store = useCompanionStore()
    expect(store.latestEventLine).not.toBe('')
    expect(COMPANION_LINES.signin).toContain(store.latestEventLine)
    expect(store.unreadCount).toBe(0)
  })

  it('does not poll while the document is hidden', async () => {
    setVisibility('hidden')
    signIn()
    mountSync()
    await flushPromises()

    expect(companionApi.getCompanion).toHaveBeenCalledTimes(1)
    expect(companionApi.listCompanionEvents).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(COMPANION_EVENT_POLL_MS)
    expect(companionApi.listCompanionEvents).not.toHaveBeenCalled()

    setVisibility('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()

    expect(companionApi.listCompanionEvents).toHaveBeenCalledTimes(1)
  })

  it('stops polling when the owning component unmounts', async () => {
    signIn()
    const wrapper = mountSync()
    await flushPromises()
    expect(companionApi.listCompanionEvents).toHaveBeenCalledTimes(1)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(COMPANION_EVENT_POLL_MS * 3)

    expect(companionApi.listCompanionEvents).toHaveBeenCalledTimes(1)
  })

  it('stops polling after logout and keeps the last remote state', async () => {
    signIn()
    mountSync()
    await flushPromises()
    expect(companionApi.listCompanionEvents).toHaveBeenCalledTimes(1)

    useAuthStore().clear()
    await flushPromises()

    companionApi.listCompanionEvents.mockResolvedValue([companionEvent(3, 'task')])
    await vi.advanceTimersByTimeAsync(COMPANION_EVENT_POLL_MS * 2)

    expect(companionApi.getCompanion).toHaveBeenCalledTimes(1)
    expect(companionApi.listCompanionEvents).toHaveBeenCalledTimes(1)
    expect(useCompanionStore().bond).toBe(120)
  })

  it('refetches when the logged-in user changes', async () => {
    signIn(7)
    mountSync()
    await flushPromises()
    expect(companionApi.getCompanion).toHaveBeenCalledTimes(1)

    signIn(8)
    await flushPromises()

    expect(companionApi.getCompanion).toHaveBeenCalledTimes(2)
  })

  it('ignores a slow response that belongs to the previous user', async () => {
    let resolveFirst!: (value: CompanionState) => void
    companionApi.getCompanion
      .mockImplementationOnce(
        () => new Promise<CompanionState>((resolve) => (resolveFirst = resolve)) as never,
      )
      .mockResolvedValueOnce(remoteState({ bond: 200, level: 4, levelName: '在意' }))

    signIn(7)
    mountSync()
    await flushPromises()
    expect(useCompanionStore().bond).toBe(0)

    signIn(8)
    await flushPromises()
    expect(useCompanionStore().bond).toBe(200)

    resolveFirst(remoteState({ bond: 10, level: 1, levelName: '认识' }))
    await flushPromises()

    const store = useCompanionStore()
    expect(store.bond).toBe(200)
    expect(store.level).toBe(4)
  })

  it('keeps local preferences and the previous remote state when requests fail', async () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    store.setActivityLevel('lively')
    store.setMotion('reduce')
    store.applyRemoteState(remoteState({ bond: 55, level: 2, unreadCount: 1 }))

    companionApi.getCompanion.mockRejectedValue(new Error('offline'))
    companionApi.listCompanionEvents.mockRejectedValue(new Error('offline'))
    companionApi.markCompanionEventsRead.mockRejectedValue(new Error('offline'))

    signIn()
    mountSync()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(COMPANION_EVENT_POLL_MS)

    expect(store.bond).toBe(55)
    expect(store.level).toBe(2)
    expect(store.unreadCount).toBe(1)
    expect(store.enabled).toBe(false)
    expect(store.activityLevel).toBe('lively')
    expect(store.motion).toBe('reduce')
  })

  it('keeps the unread count when marking events read fails', async () => {
    companionApi.listCompanionEvents.mockResolvedValue([companionEvent(4, 'achievement')])
    companionApi.markCompanionEventsRead.mockRejectedValue(new Error('offline'))

    signIn()
    mountSync()
    await flushPromises()

    const store = useCompanionStore()
    expect(store.latestEventLine).not.toBe('')
    expect(COMPANION_LINES.achievement).toContain(store.latestEventLine)
    expect(store.unreadCount).toBe(2)
  })

  it.each([
    ['signin', 'signin'],
    ['ac:daily', 'ac'],
    ['ac:first', 'ac'],
    ['task', 'task'],
    ['achievement', 'achievement'],
    ['contest', 'contest'],
    ['pet', 'pet'],
    ['streak', 'streak'],
    ['mystery', 'greeting'],
  ])('maps event type %s to the %s line key', (type, key) => {
    expect(companionEventLineKey(type)).toBe(key)
  })
})
