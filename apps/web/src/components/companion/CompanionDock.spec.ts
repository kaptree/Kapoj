import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getCompanion, petCompanion } from '@/api/companion'
import { getContest } from '@/api/contests'
import { getActivitySummary, signInActivity } from '@/api/leaderboard'
import { clearCompanionMuteCache } from '@/composables/useCompanionMute'
import { pickCompanionLine } from '@/content/companion/lines.zh-CN'
import { useCompanionStore } from '@/stores/companion'
import { localDateKey } from '@/utils/date'
import CompanionDock from './CompanionDock.vue'

const auth = vi.hoisted(() => ({ isLoggedIn: true, user: { id: 7 } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/leaderboard', () => ({
  getActivitySummary: vi.fn().mockResolvedValue({ signedInToday: true }),
  signInActivity: vi.fn(),
}))
vi.mock('@/api/contests', () => ({ getContest: vi.fn() }))
vi.mock('@/api/companion', () => ({ getCompanion: vi.fn(), petCompanion: vi.fn() }))

/** Site config with the companion switched off site-wide. */
const GLOBAL_OFF = {
  enabled: false,
  name: 'Kapo',
  defaultActivityLevel: 'normal' as const,
  contestMute: true,
  allowUserDisable: true,
}

function remoteState(overrides: Record<string, unknown> = {}) {
  return {
    bond: 120,
    level: 3,
    levelName: '搭档',
    nextThreshold: 170,
    dailyGain: 4,
    dailyCap: 15,
    activityLevel: 'normal' as const,
    dockEnabled: true,
    publicProfile: false,
    equippedSkin: '',
    unlockedSkins: [],
    unreadCount: 0,
    todayEvents: [],
    ...overrides,
  }
}

const routes = [
  { path: '/', name: 'home', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
  { path: '/contests/:id(\\d+)', name: 'contest-detail', component: { template: '<div />' } },
]

async function mountDock(path = '/') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(CompanionDock, {
    global: { plugins: [router, PrimeVue, pinia] },
    attachTo: document.body,
  })
}

/** Document-order tabbables, so Tab behavior can be asserted in jsdom. */
function tabbableElements(): HTMLElement[] {
  return Array.from(
    document.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  )
}

beforeEach(() => {
  localStorage.clear()
  auth.isLoggedIn = true
  vi.mocked(getActivitySummary).mockResolvedValue({ signedInToday: true } as never)
  vi.mocked(getContest).mockReset()
  vi.mocked(petCompanion).mockReset()
  vi.mocked(petCompanion).mockResolvedValue({ awarded: true, bond: 121, level: 3 } as never)
  vi.mocked(getCompanion).mockReset()
  vi.mocked(getCompanion).mockResolvedValue(remoteState() as never)
  clearCompanionMuteCache()
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('CompanionDock', () => {
  it('hides for guests', async () => {
    auth.isLoggedIn = false
    const wrapper = await mountDock()
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })

  it('hides on auth routes even when a session exists', async () => {
    const wrapper = await mountDock('/login')
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })

  it('opens and closes with Escape', async () => {
    const wrapper = await mountDock()
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')
    expect(wrapper.get('.companion-dock__toggle').attributes('aria-expanded')).toBe('true')
    await wrapper.get('.companion-dock__panel').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('.companion-dock__panel').exists()).toBe(false)
  })

  it('offers sign-in when today is not signed', async () => {
    vi.mocked(getActivitySummary).mockResolvedValue({ signedInToday: false } as never)
    const wrapper = await mountDock()
    await flushPromises()
    expect(wrapper.text()).toContain('签到')
    await wrapper.get('.companion-dock__signin').trigger('click')
    expect(signInActivity).toHaveBeenCalled()
  })

  it('can switch to quiet mode from the panel', async () => {
    const wrapper = await mountDock()
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')
    await wrapper.get('.companion-dock__quiet').trigger('click')
    expect(useCompanionStore().activityLevel).toBe('quiet')
  })

  it('moves keyboard focus into the panel and tabs to its controls', async () => {
    const wrapper = await mountDock()
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')
    await flushPromises()

    const panel = wrapper.get('.companion-dock__panel')
    const active = document.activeElement as HTMLElement | null
    expect(active).not.toBeNull()
    expect(panel.element === active || panel.element.contains(active)).toBe(true)

    const order = tabbableElements()
    const index = order.indexOf(active!)
    expect(index).toBeGreaterThanOrEqual(0)
    const next = order[index + 1]
    expect(next).toBeDefined()
    expect(panel.element.contains(next)).toBe(true)
  })

  it('disappears when the companion is disabled', async () => {
    const wrapper = await mountDock()
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })

  it('stays hidden on a running contest while its status is unknown and after it resolves', async () => {
    let resolveContest!: (value: unknown) => void
    vi.mocked(getContest).mockImplementation(
      () => new Promise((resolve) => (resolveContest = resolve)) as never,
    )

    const wrapper = await mountDock('/contests/7')

    // Status is still pending: the dock must not flash in.
    expect(wrapper.find('.companion-dock').exists()).toBe(false)

    resolveContest({ status: 'running' })
    await flushPromises()

    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })

  it('shows the dock on a non-running contest once its status resolves', async () => {
    vi.mocked(getContest).mockResolvedValue({ status: 'ended' } as never)

    const wrapper = await mountDock('/contests/7')
    await flushPromises()

    expect(wrapper.find('.companion-dock').exists()).toBe(true)
  })

  it('shows the bond meter inside the panel once progress is known', async () => {
    const wrapper = await mountDock()
    const store = useCompanionStore()
    store.load()
    store.applyRemoteState(remoteState() as never)
    await flushPromises()

    await wrapper.get('.companion-dock__toggle').trigger('click')

    expect(wrapper.get('.companion-bond').text()).toContain('Lv.3 搭档 120/170')
    expect(wrapper.text()).toContain('今日 4/15')
  })

  it('keeps the meter out of the panel for a local companion at bond 0', async () => {
    const wrapper = await mountDock()
    await flushPromises()

    await wrapper.get('.companion-dock__toggle').trigger('click')

    expect(wrapper.find('.companion-bond').exists()).toBe(false)
  })

  it('pets once, disables the button in flight and updates the store', async () => {
    let resolvePet!: (value: unknown) => void
    vi.mocked(petCompanion).mockImplementation(
      () => new Promise((resolve) => (resolvePet = resolve)) as never,
    )
    // A pet that levels up: the follow-up read model supplies name/threshold.
    vi.mocked(getCompanion).mockResolvedValue(
      remoteState({ bond: 121, level: 4, levelName: '搭档', nextThreshold: 220 }) as never,
    )

    const wrapper = await mountDock()
    const store = useCompanionStore()
    store.load()
    store.applyRemoteState(remoteState() as never)
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')

    const pet = wrapper.get('.companion-dock__pet')
    await pet.trigger('click')
    expect(petCompanion).toHaveBeenCalledTimes(1)
    expect(pet.attributes('disabled')).toBeDefined()

    // A second click while the request is pending must not submit again.
    await pet.trigger('click')
    expect(petCompanion).toHaveBeenCalledTimes(1)

    resolvePet({ awarded: true, bond: 121, level: 4 })
    await flushPromises()

    expect(store.bond).toBe(121)
    expect(wrapper.get('.companion-dock__pet').attributes('disabled')).toBeUndefined()
    expect(wrapper.text()).toContain(pickCompanionLine('pet', localDateKey()))
    // The refresh keeps level name and threshold in sync with the new level.
    expect(wrapper.get('.companion-bond').text()).toContain('Lv.4 搭档 121/220')
  })

  it('shows the already-petted line when the server awards nothing', async () => {
    vi.mocked(petCompanion).mockResolvedValue({ awarded: false, bond: 120, level: 3 } as never)

    const wrapper = await mountDock()
    const store = useCompanionStore()
    store.load()
    store.applyRemoteState(remoteState() as never)
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')

    await wrapper.get('.companion-dock__pet').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('今天已经摸过了')
    expect(store.bond).toBe(120)
  })

  it('keeps the local bond and re-enables petting when the request fails', async () => {
    vi.mocked(petCompanion).mockRejectedValue(new Error('offline'))

    const wrapper = await mountDock()
    const store = useCompanionStore()
    store.load()
    store.applyRemoteState(remoteState() as never)
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')

    await wrapper.get('.companion-dock__pet').trigger('click')
    await flushPromises()

    expect(store.bond).toBe(120)
    expect(wrapper.get('.companion-dock__pet').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('.companion-dock__error').exists()).toBe(true)
  })

  it('prefers the latest synced event line in the bubble', async () => {
    const wrapper = await mountDock()
    useCompanionStore().setLatestEventLine('同步来的事件台词')
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')

    expect(wrapper.get('.companion-dock__panel').text()).toContain('同步来的事件台词')
  })

  it('lets a newer synced event replace the pet line', async () => {
    const wrapper = await mountDock()
    const store = useCompanionStore()
    store.load()
    store.applyRemoteState(remoteState() as never)
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')

    await wrapper.get('.companion-dock__pet').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain(pickCompanionLine('pet', localDateKey()))

    store.setLatestEventLine('后来同步的事件')
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('后来同步的事件')
  })

  it('hides when the site disables the companion globally', async () => {
    const wrapper = await mountDock()
    await flushPromises()
    expect(wrapper.find('.companion-dock').exists()).toBe(true)

    useCompanionStore().applyGlobalConfig(GLOBAL_OFF)
    await wrapper.vm.$nextTick()

    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })
})
