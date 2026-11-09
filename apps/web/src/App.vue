<script setup lang="ts">
import { onMounted } from 'vue'
import ConfirmDialog from 'primevue/confirmdialog'
import Toast from 'primevue/toast'

import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'

const auth = useAuthStore()
const site = useSiteStore()

// Refresh the cached user so role or profile changes made elsewhere show up.
onMounted(() => {
  void site.ensureLoaded()
  if (auth.isLoggedIn) void auth.fetchMe().catch(() => undefined)
})
</script>

<template>
  <RouterView />
  <Toast position="top-right" />
  <ConfirmDialog />
</template>
