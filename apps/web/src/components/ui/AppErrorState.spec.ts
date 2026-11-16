import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it } from 'vitest'
import type { CompanionExpression } from '@/content/companion/types'
import { useCompanionStore } from '@/stores/companion'
import AppErrorState from './AppErrorState.vue'

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

function mountError(props: { mascot?: CompanionExpression | '' } = {}) {
  return mount(AppErrorState, {
    props: { description: '请稍后再试。', ...props },
    global: { plugins: [PrimeVue] },
  })
}

describe('AppErrorState', () => {
  it('shows an annoyed mascot when requested', () => {
    const wrapper = mountError({ mascot: 'annoyed' })
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
  })

  it('keeps the warning icon when no mascot is set', () => {
    const wrapper = mountError({})
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(false)
    expect(wrapper.find('.pi-exclamation-triangle').exists()).toBe(true)
  })

  it('hides the mascot when the companion is disabled', () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    const wrapper = mountError({ mascot: 'annoyed' })
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(false)
    expect(wrapper.find('.p-button').exists()).toBe(true)
  })
})
