import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it } from 'vitest'
import { useCompanionStore } from '@/stores/companion'
import CompanionSettings from './CompanionSettings.vue'

let pinia: ReturnType<typeof createPinia>

function mountSettings() {
  pinia = createPinia()
  setActivePinia(pinia)
  return mount(CompanionSettings, { global: { plugins: [PrimeVue, pinia] } })
}

beforeEach(() => {
  localStorage.clear()
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
})
