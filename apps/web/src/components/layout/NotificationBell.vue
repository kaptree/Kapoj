<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from 'vue'

import { useAuthStore } from '@/stores/auth'
import { useNotificationsStore } from '@/stores/notifications'

const POLL_INTERVAL = 60_000

const auth = useAuthStore()
const notifications = useNotificationsStore()
let timer: ReturnType<typeof setInterval> | null = null

const badgeValue = computed(() =>
  notifications.unreadCount > 99 ? '99+' : String(notifications.unreadCount),
)

function refresh() {
  if (!auth.isLoggedIn) return
  void notifications.refreshUnread()
}

onMounted(() => {
  refresh()
  timer = setInterval(refresh, POLL_INTERVAL)
})

onBeforeUnmount(() => {
  if (timer !== null) clearInterval(timer)
  timer = null
})
</script>

<template>
  <RouterLink
    v-if="auth.isLoggedIn"
    to="/notifications"
    class="notification-bell"
    aria-label="通知"
  >
    <i class="pi pi-bell" aria-hidden="true" />
    <span v-if="notifications.unreadCount > 0" class="notification-bell__badge">{{
      badgeValue
    }}</span>
  </RouterLink>
</template>

<style scoped>
.notification-bell {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  color: var(--kapoj-text-secondary);
  border-radius: var(--kapoj-radius-sm);
  text-decoration: none;
}

.notification-bell:hover {
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.notification-bell__badge {
  position: absolute;
  top: 1px;
  right: 1px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 15px;
  height: 15px;
  padding: 0 4px;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
  background: var(--kapoj-danger);
  border-radius: 999px;
}
</style>
