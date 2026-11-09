import { flushPromises, mount } from '@vue/test-utils'
import { h } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it } from 'vitest'

import AppShell from './AppShell.vue'

const routes = [
  { path: '/', component: { template: '<div />' } },
  { path: '/demo', component: { template: '<div />' }, meta: { title: '演示标题' } },
]

async function mountShell(props: Record<string, unknown> = {}) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/demo')
  await router.isReady()
  const wrapper = mount(AppShell, {
    props: {
      titleFallback: '控制台',
      kicker: 'KAPOJ // TEST',
      ...props,
    },
    slots: {
      nav: ({ navigate }: { navigate: () => void }) =>
        h('button', { id: 'nav-close', onClick: navigate }, '导航项'),
      default: () => h('div', { class: 'shell-child' }, '内容'),
      footer: () => h('footer', { class: 'test-footer' }, '页脚'),
    },
    global: {
      plugins: [router, PrimeVue, createPinia()],
      stubs: { UserMenu: true, AppThemeToggle: true, NotificationBell: true },
    },
  })
  return { wrapper, router }
}

beforeEach(() => {
  document.body.innerHTML = ''
})

describe('AppShell', () => {
  it('renders the shared shell, nav slot, page title and content', async () => {
    const { wrapper } = await mountShell()

    expect(wrapper.find('.app-shell').exists()).toBe(true)
    expect(wrapper.text()).toContain('KAPOJ // TEST')
    expect(wrapper.text()).toContain('演示标题')
    expect(wrapper.find('.shell-child').text()).toBe('内容')
    expect(wrapper.find('#app-sidebar-nav').exists()).toBe(true)
  })

  it('wraps the footer slot in a div so only the slotted footer is a landmark', async () => {
    const { wrapper } = await mountShell()
    const appFooter = wrapper.find('.app-footer')

    expect(appFooter.element.tagName).toBe('DIV')
    expect(appFooter.find('footer.test-footer').exists()).toBe(true)
    expect(wrapper.findAll('footer')).toHaveLength(1)
  })

  it('supports the admin brand badge and sidebar footer copy', async () => {
    const { wrapper } = await mountShell({
      brandBadge: '管理后台',
      sidebarFooterText: '管理控制台',
      sidebarVersion: 'M4-D',
    })

    expect(wrapper.text()).toContain('管理后台')
    expect(wrapper.text()).toContain('管理控制台')
    expect(wrapper.text()).toContain('M4-D')
  })

  it('opens the mobile drawer and closes it when the nav slot navigates', async () => {
    const { wrapper } = await mountShell()
    const toggle = wrapper.find('.app-menu-toggle')

    expect(toggle.attributes('aria-expanded')).toBe('false')

    await toggle.trigger('click')
    await flushPromises()

    expect(toggle.attributes('aria-expanded')).toBe('true')
    const mobileNav = document.body.querySelector('#app-mobile-nav')
    expect(mobileNav).toBeTruthy()

    const closeButton = mobileNav!.querySelector('#nav-close') as HTMLElement
    closeButton.click()
    await flushPromises()

    expect(toggle.attributes('aria-expanded')).toBe('false')
  })

  it('hides the user menu when showUserMenu is false', async () => {
    const { wrapper } = await mountShell({ showUserMenu: false })

    expect(wrapper.find('user-menu-stub').exists()).toBe(false)
    expect(wrapper.find('notification-bell-stub').exists()).toBe(false)
  })

  it('renders the notification bell before the user menu', async () => {
    const { wrapper } = await mountShell()

    const bell = wrapper.find('notification-bell-stub')
    const menu = wrapper.find('user-menu-stub')
    expect(bell.exists()).toBe(true)
    expect(menu.exists()).toBe(true)
    expect(wrapper.html().indexOf('notification-bell-stub')).toBeLessThan(
      wrapper.html().indexOf('user-menu-stub'),
    )
  })
})
