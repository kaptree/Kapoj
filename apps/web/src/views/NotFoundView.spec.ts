import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'

import { router } from '@/router'
import NotFoundView from './NotFoundView.vue'
import notFoundSource from './NotFoundView.vue?raw'

function mountNotFound() {
  const memoryRouter = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', name: 'home', component: { template: '<div />' } }],
  })
  return mount(NotFoundView, { global: { plugins: [memoryRouter, PrimeVue] } })
}

describe('NotFoundView', () => {
  it('renders the 404 code, title, description and a home link', () => {
    const wrapper = mountNotFound()

    expect(wrapper.text()).toContain('404')
    expect(wrapper.text()).toContain('页面不存在')
    expect(wrapper.text()).toContain('请检查地址是否正确。')
    expect(wrapper.find('a[href="/"]').exists()).toBe(true)
  })

  it('renders no Element Plus markup or imports', () => {
    const wrapper = mountNotFound()

    expect(wrapper.find('.el-result').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('el-result')
    expect(wrapper.html()).not.toContain('el-button')
    expect(notFoundSource).not.toMatch(/element-plus|<el-/)
  })

  it('registers the not-found route with a title', () => {
    const notFound = router.getRoutes().find((route) => route.name === 'not-found')

    expect(notFound?.meta.title).toBe('页面不存在')
  })
})
