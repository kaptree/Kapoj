<script setup lang="ts">
import { onMounted } from 'vue'

import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()

// Refresh the cached user so role or profile changes made elsewhere show up.
onMounted(() => {
  if (auth.isLoggedIn) void auth.fetchMe().catch(() => undefined)
})
</script>

<template>
  <RouterView />
</template>
