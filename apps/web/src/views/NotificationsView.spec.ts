import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Paginator from 'primevue/paginator'
import ToastService from 'primevue/toastservice'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import { listNotifications, markAllRead, markRead } from '@/api/social'
import type { NotificationItem, SocialUserSummary } from '@/api/types'
import NotificationsView from './NotificationsView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const routerPush = vi.hoisted(() => vi.fn())

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRouter: () => ({ push: routerPush }) }
})
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/api/social', () => ({
  listNotifications: vi.fn(),
  markRead: vi.fn(),
  markAllRead: vi.fn(),
}))

function actor(nickname: string): SocialUserSummary {
  return { id: 2, username: nickname.toLowerCase(), nickname, avatarUrl: '', rating: 1500 }
}

function notification(overrides: Partial<NotificationItem> = {}): NotificationItem {
  return {
    id: 1,
    type: 'solution_like',
    actor: actor('Bob'),
    targetType: 'solution_post',
    targetId: 9,
    link: '/problems/1/solutions/9',
    readAt: null,
    createdAt: '2026-01-02T03:04:05Z',
    ...overrides,
  }
}

function mountView() {
  return mount(NotificationsView, {
    global: {
      plugins: [PrimeVue, ToastService, createPinia()],
    },
  })
}

function button(wrapper: ReturnType<typeof mountView>, label: string) {
  return wrapper.findAll('button').find((b) => b.text().trim() === label)
}

beforeEach(() => {
  toast.add.mockReset()
  routerPush.mockReset()
  vi.mocked(listNotifications).mockReset()
  vi.mocked(markRead).mockReset()
  vi.mocked(markAllRead).mockReset()
})

describe('NotificationsView', () => {
  it('lists notifications with type labels, actor and time', async () => {
    vi.mocked(listNotifications).mockResolvedValue({
      total: 3,
      items: [
        notification({ id: 1, type: 'solution_reply', actor: actor('Alice') }),
        notification({ id: 2, type: 'follow', actor: actor('Carol') }),
        notification({ id: 3, type: 'solution_like', actor: null }),
      ],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(listNotifications).toHaveBeenCalledWith({ page: 1, pageSize: 20 })
    expect(wrapper.text()).toContain('回复了你的解题思路')
    expect(wrapper.text()).toContain('关注了你')
    expect(wrapper.text()).toContain('赞了你的解题思路')
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('Carol')
    expect(wrapper.text()).toContain('未知用户')
    wrapper.unmount()
  })

  it('marks an unread notification read and navigates to its link', async () => {
    vi.mocked(listNotifications).mockResolvedValue({
      total: 1,
      items: [notification({ readAt: null })],
    })
    vi.mocked(markRead).mockResolvedValue(null)
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('button.notification-item').trigger('click')
    await flushPromises()

    expect(markRead).toHaveBeenCalledWith(1)
    expect(routerPush).toHaveBeenCalledWith('/problems/1/solutions/9')
    wrapper.unmount()
  })

  it('navigates without marking an already-read notification', async () => {
    vi.mocked(listNotifications).mockResolvedValue({
      total: 1,
      items: [notification({ readAt: '2026-01-02T04:00:00Z' })],
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('button.notification-item').trigger('click')
    await flushPromises()

    expect(markRead).not.toHaveBeenCalled()
    expect(routerPush).toHaveBeenCalledWith('/problems/1/solutions/9')
    wrapper.unmount()
  })

  it('marks all notifications as read', async () => {
    vi.mocked(listNotifications).mockResolvedValue({
      total: 1,
      items: [notification({ readAt: null })],
    })
    vi.mocked(markAllRead).mockResolvedValue(null)
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('.notification-item--unread').exists()).toBe(true)

    await button(wrapper, '全部已读')!.trigger('click')
    await flushPromises()

    expect(markAllRead).toHaveBeenCalled()
    expect(wrapper.find('.notification-item--unread').exists()).toBe(false)
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('shows an empty state', async () => {
    vi.mocked(listNotifications).mockResolvedValue({ total: 0, items: [] })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('暂无通知')
    wrapper.unmount()
  })

  it('shows an error and retries', async () => {
    vi.mocked(listNotifications)
      .mockRejectedValueOnce(new ApiError(500, 50000, '加载通知失败'))
      .mockResolvedValueOnce({ total: 0, items: [] })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('加载通知失败')
    const retry = button(wrapper, '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()

    expect(listNotifications).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('paginates', async () => {
    vi.mocked(listNotifications).mockResolvedValue({ total: 45, items: [notification()] })
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(listNotifications).toHaveBeenLastCalledWith({ page: 2, pageSize: 20 })
    wrapper.unmount()
  })
})
