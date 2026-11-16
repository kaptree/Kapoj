import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { useCompanionStore } from '@/stores/companion'
import AppLoadingState from './AppLoadingState.vue'

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

describe('AppLoadingState', () => {
  it('shows a chibi when a mascot is provided', () => {
    const wrapper = mount(AppLoadingState, { props: { mascot: 'sleepy' } })
    expect(wrapper.find('.companion-avatar__image').exists()).toBe(true)
  })

  it('shows only the spinner and label without a mascot', () => {
    const wrapper = mount(AppLoadingState, { props: { label: '加载中' } })
    expect(wrapper.find('.companion-avatar').exists()).toBe(false)
    expect(wrapper.find('.app-loading__spinner').exists()).toBe(true)
    expect(wrapper.find('.app-loading__label').text()).toBe('加载中')
  })

  it('hides the mascot when the companion is disabled', () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    const wrapper = mount(AppLoadingState, { props: { mascot: 'sleepy' } })
    expect(wrapper.find('.companion-avatar').exists()).toBe(false)
    expect(wrapper.find('.app-loading__spinner').exists()).toBe(true)
  })
})
