<script setup lang="ts">
import { onMounted, watch } from 'vue'
import ConfirmDialog from 'primevue/confirmdialog'
import Toast from 'primevue/toast'
import { useToast } from 'primevue/usetoast'

import { signInActivity } from '@/api/leaderboard'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'

const auth = useAuthStore()
const site = useSiteStore()
const toast = useToast()

let signInInFlight = false
const attemptedSignInKeys = new Set<string>()

function localDateKey(): string {
  const now = new Date()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  return `${now.getFullYear()}-${month}-${day}`
}

async function autoSignIn() {
  const userID = auth.user?.id
  if (!auth.isLoggedIn || !userID || signInInFlight) return
  const key = `${userID}:${localDateKey()}`
  if (attemptedSignInKeys.has(key)) return
  attemptedSignInKeys.add(key)
  signInInFlight = true
  try {
    const result = await signInActivity()
    if (result.awarded) {
      toast.add({
        severity: 'success',
        summary: '签到成功',
        detail: `活跃度 +${result.activityPoints} · 金币 +${result.coins}`,
        life: 4000,
      })
      await auth.fetchMe().catch(() => undefined)
    }
  } catch {
    // 后台签到失败不打扰用户；本次运行不再重试，下次启动会再次尝试。
  } finally {
    signInInFlight = false
    // A login can land while another user's sign-in is in flight; make sure the
    // new session gets its own attempt.
    const nextID = auth.user?.id
    if (nextID && !attemptedSignInKeys.has(`${nextID}:${localDateKey()}`)) {
      void autoSignIn()
    }
  }
}

// Refresh the cached user so role or profile changes made elsewhere show up.
onMounted(() => {
  void site.ensureLoaded()
  if (auth.isLoggedIn) void auth.fetchMe().catch(() => undefined)
})

watch(
  () => auth.user?.id,
  () => void autoSignIn(),
  { immediate: true },
)
</script>

<template>
  <RouterView />
  <Toast position="top-right" />
  <ConfirmDialog />
</template>
