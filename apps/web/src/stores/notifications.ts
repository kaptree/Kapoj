import { defineStore } from 'pinia'
import { ref } from 'vue'

import {
  markAllRead as apiMarkAllRead,
  markRead as apiMarkRead,
  unreadCount as apiUnreadCount,
} from '@/api/social'

/**
 * Shared unread-notification state. The bell and the notifications page both
 * use this store so marking a notification read updates the badge immediately.
 * Polling remains a fallback, not the only source of truth.
 */
export const useNotificationsStore = defineStore('notifications', () => {
  const unreadCount = ref(0)
  let refreshInFlight: Promise<void> | null = null

  async function refreshUnread() {
    if (refreshInFlight) return refreshInFlight
    refreshInFlight = (async () => {
      try {
        const result = await apiUnreadCount()
        unreadCount.value = result.count
      } catch {
        // Keep the previous value; background polling must not clear the badge.
      } finally {
        refreshInFlight = null
      }
    })()
    return refreshInFlight
  }

  async function markRead(id: number | string, wasUnread = true) {
    await apiMarkRead(id)
    if (wasUnread) {
      unreadCount.value = Math.max(0, unreadCount.value - 1)
    }
  }

  async function markAllRead() {
    await apiMarkAllRead()
    unreadCount.value = 0
  }

  return { unreadCount, refreshUnread, markRead, markAllRead }
})
