import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import type { Me } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import GamificationAdminView from './GamificationAdminView.vue'

const ALL_GAMIFICATION = [
  'gamification.achievement.manage',
  'gamification.task.manage',
  'gamification.badge.manage',
  'gamification.title.manage',
]

function me(permissions: string[], isSuperAdmin = false): Me {
  return {
    id: 1,
    username: 'admin',
    nickname: '管理员',
    email: 'admin@example.com',
    avatarUrl: '',
    avatarUploaded: false,
    bio: '',
    role: 'user',
    roles: [],
    permissions,
    isSuperAdmin,
    rating: 1800,
    coins: 0,
    status: 'active',
    createdAt: '2025-01-01T00:00:00Z',
  }
}

async function mountPage(permissions: string[], isSuperAdmin = false) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.setTokens({ accessToken: 'access', refreshToken: 'refresh' })
  auth.setUser(me(permissions, isSuperAdmin))
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
  it('no gamification permission shows no-permission state', async () => {
    const { wrapper } = await mountPage([])

    expect(wrapper.text()).toContain('没有权限')
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(0)
    wrapper.unmount()
  })

  it('shows only permitted domain tabs', async () => {
    const { wrapper } = await mountPage(['gamification.task.manage', 'gamification.title.manage'])

    expect(wrapper.findAll('[role="tab"]').map((tab) => tab.text())).toEqual(['任务', '称号'])
    wrapper.unmount()
  })

  it('shows the four admin tabs with lazy panels', async () => {
    const { wrapper } = await mountPage(ALL_GAMIFICATION)

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

  it('super admin sees every tab', async () => {
    const { wrapper } = await mountPage([], true)
    expect(wrapper.findAll('[role="tab"]').map((tab) => tab.text())).toEqual([
      '成就',
      '任务',
      '徽章',
      '称号',
    ])
    wrapper.unmount()
  })
})
