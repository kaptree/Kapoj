import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it } from 'vitest'

import { useCompanionStore } from '@/stores/companion'
import AuthLayout from './AuthLayout.vue'

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }],
  })
}

beforeEach(() => {
  localStorage.clear()
})

describe('AuthLayout', () => {
  it('renders static companion art without a dock', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: { template: '<div />' } }],
    })
    await router.push('/')
    await router.isReady()
    const wrapper = mount(AuthLayout, {
      props: { title: '登录' },
      global: { plugins: [router, createPinia()] },
    })
    expect(wrapper.find('.auth-intro__companion').exists()).toBe(true)
    expect(wrapper.find('.auth-panel__companion').exists()).toBe(true)
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
    expect(wrapper.find('.auth-intro__companion button').exists()).toBe(false)
  })

  it('hides both companion blocks when the companion is disabled', async () => {
    const router = makeRouter()
    await router.push('/')
    await router.isReady()

    const pinia = createPinia()
    setActivePinia(pinia)
    const companion = useCompanionStore()
    companion.load()
    companion.setEnabled(false)

    const wrapper = mount(AuthLayout, {
      props: { title: '登录' },
      global: { plugins: [router, pinia] },
    })
    expect(wrapper.find('.auth-intro__companion').exists()).toBe(false)
    expect(wrapper.find('.auth-panel__companion').exists()).toBe(false)
  })
})
