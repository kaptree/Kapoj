<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import AppShell from '@/components/layout/AppShell.vue'
import AppNav from '@/components/layout/AppNav.vue'

const route = useRoute()
const isAuthRoute = computed(() => route.name === 'login' || route.name === 'register')
</script>

<template>
  <AppShell title-fallback="控制台" kicker="KAPOJ // CONSOLE" :show-user-menu="!isAuthRoute">
    <template #nav="{ navigate }">
      <AppNav @navigate="navigate" />
    </template>
    <RouterView />
  </AppShell>
</template>
