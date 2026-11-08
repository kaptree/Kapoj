import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { unreadCount } from '@/api/social'
import NotificationBell from './NotificationBell.vue'

const auth = vi.hoisted(() => ({ isLoggedIn: true }))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/social', () => ({
  unreadCount: vi.fn(),
}))

function mountBell() {
  return mount(NotificationBell, {
    global: {
      stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } },
    },
  })
}

beforeEach(() => {
  auth.isLoggedIn = true
  vi.mocked(unreadCount).mockReset()
  vi.mocked(unreadCount).mockResolvedValue({ count: 0 })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('NotificationBell', () => {
  it('renders nothing for guests and does not fetch', async () => {
    auth.isLoggedIn = false
    const wrapper = mountBell()
    await flushPromises()

    expect(wrapper.find('a.notification-bell').exists()).toBe(false)
    expect(unreadCount).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('fetches the unread count on mount and shows the badge', async () => {
    vi.mocked(unreadCount).mockResolvedValue({ count: 3 })
    const wrapper = mountBell()
    await flushPromises()

    expect(unreadCount).toHaveBeenCalledTimes(1)
    const link = wrapper.find('a.notification-bell')
    expect(link.exists()).toBe(true)
    expect(link.attributes('href')).toBe('/notifications')
    expect(wrapper.find('.notification-bell__badge').text()).toBe('3')
    wrapper.unmount()
  })

  it('hides the badge when there are no unread notifications', async () => {
    vi.mocked(unreadCount).mockResolvedValue({ count: 0 })
    const wrapper = mountBell()
    await flushPromises()

    expect(wrapper.find('.notification-bell__badge').exists()).toBe(false)
    wrapper.unmount()
  })

  it('polls every 60 seconds and clears the timer on unmount', async () => {
    const setIntervalSpy = vi.spyOn(globalThis, 'setInterval')
    const clearIntervalSpy = vi.spyOn(globalThis, 'clearInterval')
    vi.mocked(unreadCount).mockResolvedValue({ count: 1 })

    const wrapper = mountBell()
    await flushPromises()
    expect(unreadCount).toHaveBeenCalledTimes(1)

    expect(setIntervalSpy).toHaveBeenCalledWith(expect.any(Function), 60_000)
    const callback = setIntervalSpy.mock.calls[0]![0] as () => void
    callback()
    await flushPromises()
    expect(unreadCount).toHaveBeenCalledTimes(2)

    wrapper.unmount()
    expect(clearIntervalSpy).toHaveBeenCalled()
  })
})
