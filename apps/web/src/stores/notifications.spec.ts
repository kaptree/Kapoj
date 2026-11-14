import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { markAllRead, markRead, unreadCount } from '@/api/social'
import { useNotificationsStore } from './notifications'

vi.mock('@/api/social', () => ({
  unreadCount: vi.fn(),
  markRead: vi.fn(),
  markAllRead: vi.fn(),
}))

beforeEach(() => {
  setActivePinia(createPinia())
  vi.mocked(unreadCount).mockReset()
  vi.mocked(markRead).mockReset()
  vi.mocked(markAllRead).mockReset()
})

describe('notifications store', () => {
  it('refreshes the unread count', async () => {
    vi.mocked(unreadCount).mockResolvedValue({ count: 3 })
    const store = useNotificationsStore()

    await store.refreshUnread()

    expect(store.unreadCount).toBe(3)
  })

  it('keeps the previous count when refreshing fails', async () => {
    vi.mocked(unreadCount).mockRejectedValue(new Error('offline'))
    const store = useNotificationsStore()
    store.unreadCount = 2

    await store.refreshUnread()

    expect(store.unreadCount).toBe(2)
  })

  it('decrements once after marking an unread notification read', async () => {
    vi.mocked(markRead).mockResolvedValue(null)
    const store = useNotificationsStore()
    store.unreadCount = 2

    await store.markRead(9, true)

    expect(markRead).toHaveBeenCalledWith(9)
    expect(store.unreadCount).toBe(1)
  })

  it('does not decrement for an already-read notification', async () => {
    vi.mocked(markRead).mockResolvedValue(null)
    const store = useNotificationsStore()
    store.unreadCount = 2

    await store.markRead(9, false)

    expect(store.unreadCount).toBe(2)
  })

  it('keeps the count when marking read fails', async () => {
    vi.mocked(markRead).mockRejectedValue(new Error('offline'))
    const store = useNotificationsStore()
    store.unreadCount = 2

    await expect(store.markRead(9, true)).rejects.toThrow('offline')

    expect(store.unreadCount).toBe(2)
  })

  it('clears the unread count after marking all read', async () => {
    vi.mocked(markAllRead).mockResolvedValue(null)
    const store = useNotificationsStore()
    store.unreadCount = 4

    await store.markAllRead()

    expect(store.unreadCount).toBe(0)
  })

  it('keeps the count when marking all read fails', async () => {
    vi.mocked(markAllRead).mockRejectedValue(new Error('offline'))
    const store = useNotificationsStore()
    store.unreadCount = 4

    await expect(store.markAllRead()).rejects.toThrow('offline')

    expect(store.unreadCount).toBe(4)
  })
})
