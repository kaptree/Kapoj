import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import type { Me } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import GamificationAdminView from './GamificationAdminView.vue'

function me(role: Me['role']): Me {
  return {
    id: 1,
    username: 'admin',
    nickname: '管理员',
    email: 'admin@example.com',
    avatarUrl: '',
    bio: '',
    role,
    rating: 1800,
    coins: 0,
    status: 'active',
    createdAt: '2025-01-01T00:00:00Z',
  }
}

async function mountPage(role: Me['role']) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.setTokens({ accessToken: 'access', refreshToken: 'refresh' })
  auth.setUser(me(role))
  const wrapper = mount(GamificationAdminView, {
    global: {
      plugins: [PrimeVue, pinia],
      stubs: {
        AchievementAdminPanel: { template: '<div class="stub-achievements">成就面板</div>' },
        TaskAdminPanel: { template: '<div class="stub-tasks">任务面板</div>' },
        BadgeAdminPanel: { template: '<div class="stub-badges">徽章面板</div>' },
        TitleAdminPanel: { template: '<div class="stub-titles">称号面板</div>' },
      },
    },
  })
  await flushPromises()
  return { wrapper }
}

beforeEach(() => {
  localStorage.clear()
})

describe('GamificationAdminView', () => {
  it('blocks non super admins', async () => {
    const { wrapper } = await mountPage('contest_admin')

    expect(wrapper.text()).toContain('没有权限')
    expect(wrapper.text()).toContain('仅超级管理员可以访问游戏化管理')
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(0)
    wrapper.unmount()
  })

  it('shows the four admin tabs with lazy panels', async () => {
    const { wrapper } = await mountPage('super_admin')

    expect(wrapper.text()).toContain('游戏化管理')
    expect(wrapper.findAll('[role="tab"]').map((tab) => tab.text())).toEqual([
      '成就',
      '任务',
      '徽章',
      '称号',
    ])
    expect(wrapper.find('.stub-achievements').exists()).toBe(true)
    expect(wrapper.find('.stub-tasks').exists()).toBe(false)

    await wrapper.findAll('[role="tab"]')[1]!.trigger('click')
    await flushPromises()
    expect(wrapper.find('.stub-tasks').exists()).toBe(true)

    await wrapper.findAll('[role="tab"]')[2]!.trigger('click')
    await flushPromises()
    expect(wrapper.find('.stub-badges').exists()).toBe(true)

    await wrapper.findAll('[role="tab"]')[3]!.trigger('click')
    await flushPromises()
    expect(wrapper.find('.stub-titles').exists()).toBe(true)
    wrapper.unmount()
  })
})
