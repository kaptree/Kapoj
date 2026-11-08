<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import { unreadCount } from '@/api/social'
import { useAuthStore } from '@/stores/auth'

const POLL_INTERVAL = 60_000

const auth = useAuthStore()
const count = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

const badgeValue = computed(() => (count.value > 99 ? '99+' : String(count.value)))

async function refresh() {
  if (!auth.isLoggedIn) return
  try {
    const data = await unreadCount()
    count.value = data.count
  } catch {
    // Keep the previous count; polling errors are intentionally silent.
  }
}

onMounted(() => {
  void refresh()
  timer = setInterval(() => void refresh(), POLL_INTERVAL)
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
    <span v-if="count > 0" class="notification-bell__badge">{{ badgeValue }}</span>
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
