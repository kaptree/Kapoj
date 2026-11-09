<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'

import AppShell from '@/components/layout/AppShell.vue'
import AppNav from '@/components/layout/AppNav.vue'
import SiteFooter from '@/components/layout/SiteFooter.vue'
import { useSiteStore } from '@/stores/site'

const route = useRoute()
const site = useSiteStore()
const isAuthRoute = computed(() => route.name === 'login' || route.name === 'register')

onMounted(() => {
  void site.recordVisit()
})
</script>

<template>
  <AppShell title-fallback="控制台" kicker="KAPOJ // CONSOLE" :show-user-menu="!isAuthRoute">
    <template #nav="{ navigate }">
      <AppNav @navigate="navigate" />
    </template>
    <RouterView />
    <template #footer>
      <SiteFooter />
    </template>
  </AppShell>
</template>
