import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { updateCompanionSettings } from '@/api/companion'
import { useCompanionStore } from '@/stores/companion'
import CompanionSettings from './CompanionSettings.vue'

const auth = vi.hoisted(() => ({ isLoggedIn: false }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/companion', () => ({ updateCompanionSettings: vi.fn() }))

/** Site config with the companion switched off site-wide. */
const GLOBAL_OFF = {
  enabled: false,
  name: 'Kapo',
  defaultActivityLevel: 'normal' as const,
  contestMute: true,
  allowUserDisable: true,
}

let pinia: ReturnType<typeof createPinia>

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

function mountSettings(before?: (store: ReturnType<typeof useCompanionStore>) => void) {
  pinia = createPinia()
  setActivePinia(pinia)
  const store = useCompanionStore()
  before?.(store)
  return mount(CompanionSettings, { global: { plugins: [PrimeVue, pinia] } })
}

beforeEach(() => {
  localStorage.clear()
  auth.isLoggedIn = false
  vi.mocked(updateCompanionSettings).mockReset()
  // Mirror the server: the partial patch is merged into the returned state.
  vi.mocked(updateCompanionSettings).mockImplementation((patch) =>
    Promise.resolve({ ...remoteState(), ...patch } as never),
  )
})

describe('CompanionSettings', () => {
  it('toggles the companion and persists the choice', async () => {
    const wrapper = mountSettings()
    await wrapper.get('[data-test="companion-enabled"]').trigger('click')
    const store = useCompanionStore()
    expect(store.enabled).toBe(false)
    expect(localStorage.getItem('kapoj.companion.prefs')).toContain('false')
  })

  it('selects quiet activity and reduced motion', async () => {
    const wrapper = mountSettings()
    await wrapper.get('[data-test="activity-quiet"]').trigger('click')
    await wrapper.get('[data-test="motion-reduce"]').trigger('click')
    const store = useCompanionStore()
    expect(store.activityLevel).toBe('quiet')
    expect(store.motion).toBe('reduce')
  })

  it('restores defaults and clears stored preferences on reset', async () => {
    const wrapper = mountSettings()
    await wrapper.get('[data-test="activity-lively"]').trigger('click')
    await wrapper.get('[data-test="companion-reset"]').trigger('click')
    const store = useCompanionStore()
    expect(store.activityLevel).toBe('normal')
    expect(wrapper.get('[data-test="activity-normal"]').attributes('aria-pressed')).toBe('true')
    expect(localStorage.getItem('kapoj.companion.prefs')).toBeNull()
  })

  it('persists dock, activity and profile choices remotely when logged in', async () => {
    auth.isLoggedIn = true
    const wrapper = mountSettings()

    await wrapper.get('[data-test="activity-quiet"]').trigger('click')
    expect(updateCompanionSettings).toHaveBeenCalledWith({ activityLevel: 'quiet' })

    await wrapper.get('[data-test="companion-enabled"]').trigger('click')
    expect(updateCompanionSettings).toHaveBeenCalledWith({ dockEnabled: false })

    await wrapper.get('[data-test="public-profile"]').trigger('click')
    expect(updateCompanionSettings).toHaveBeenCalledWith({ publicProfile: true })
    expect(useCompanionStore().publicProfile).toBe(true)

    await flushPromises()
  })

  it('offers the public profile toggle only to signed-in users', () => {
    expect(mountSettings().find('[data-test="public-profile"]').exists()).toBe(false)

    auth.isLoggedIn = true
    expect(mountSettings().find('[data-test="public-profile"]').exists()).toBe(true)
  })

  it('reflects the remote public profile flag with an aria-pressed toggle', () => {
    auth.isLoggedIn = true
    const wrapper = mountSettings((store) =>
      store.applyRemoteState(remoteState({ publicProfile: true }) as never),
    )

    expect(wrapper.get('[data-test="public-profile"]').attributes('aria-pressed')).toBe('true')
  })

  it('keeps local preferences when the remote update fails', async () => {
    auth.isLoggedIn = true
    vi.mocked(updateCompanionSettings).mockRejectedValue(new Error('offline'))

    const wrapper = mountSettings()
    await wrapper.get('[data-test="activity-lively"]').trigger('click')
    await flushPromises()

    expect(useCompanionStore().activityLevel).toBe('lively')
    expect(wrapper.get('[data-test="activity-lively"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.text()).toContain('未能同步')
  })

  it('rolls back the public profile and reports an accurate error when the request fails', async () => {
    auth.isLoggedIn = true
    vi.mocked(updateCompanionSettings).mockRejectedValue(new Error('offline'))

    const wrapper = mountSettings((store) =>
      store.applyRemoteState(remoteState({ publicProfile: true }) as never),
    )
    const toggle = wrapper.get('[data-test="public-profile"]')
    expect(toggle.attributes('aria-pressed')).toBe('true')

    await toggle.trigger('click')
    await flushPromises()

    // The optimistic false must be reverted: the server never stored it.
    expect(useCompanionStore().publicProfile).toBe(true)
    expect(wrapper.get('[data-test="public-profile"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.text()).toContain('公开设置同步失败，已恢复原值')
    expect(wrapper.text()).not.toContain('本地偏好已保存')
  })

  it('never calls the API for guests and keeps the local fallback', async () => {
    const wrapper = mountSettings()
    await wrapper.get('[data-test="activity-quiet"]').trigger('click')

    expect(updateCompanionSettings).not.toHaveBeenCalled()
    expect(useCompanionStore().activityLevel).toBe('quiet')
  })

  it('keeps the settings section reachable when the site disables the companion', () => {
    const wrapper = mountSettings((store) => store.applyGlobalConfig(GLOBAL_OFF))

    expect(wrapper.find('.companion-settings').exists()).toBe(true)
    expect(wrapper.text()).toContain('站点已关闭伙伴功能')
  })
})
