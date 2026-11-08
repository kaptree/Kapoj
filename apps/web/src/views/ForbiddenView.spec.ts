import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'

import ForbiddenView from './ForbiddenView.vue'
import forbiddenSource from './ForbiddenView.vue?raw'

function mountForbidden() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', name: 'home', component: { template: '<div />' } }],
  })
  return mount(ForbiddenView, { global: { plugins: [router, PrimeVue] } })
}

describe('ForbiddenView', () => {
  it('renders the 403 code, title, description and a home link', () => {
    const wrapper = mountForbidden()

    expect(wrapper.text()).toContain('403')
    expect(wrapper.text()).toContain('没有权限')
    expect(wrapper.text()).toContain('你的账号无法访问这个页面。')
    expect(wrapper.find('a[href="/"]').exists()).toBe(true)
  })

  it('renders no Element Plus markup or imports', () => {
    const wrapper = mountForbidden()

    expect(wrapper.find('.el-result').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('el-result')
    expect(wrapper.html()).not.toContain('el-button')
    expect(forbiddenSource).not.toMatch(/element-plus|<el-/)
  })
})
