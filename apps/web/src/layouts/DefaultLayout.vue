<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'

import CompanionDock from '@/components/companion/CompanionDock.vue'
import CompanionPageNote from '@/components/companion/CompanionPageNote.vue'
import AppNav from '@/components/layout/AppNav.vue'
import AppShell from '@/components/layout/AppShell.vue'
import SiteFooter from '@/components/layout/SiteFooter.vue'
import { useCompanionMute } from '@/composables/useCompanionMute'
import { useSiteStore } from '@/stores/site'

const route = useRoute()
const site = useSiteStore()
const { muted } = useCompanionMute()
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
    <p v-if="muted" class="companion-focus-chip">专注模式 · 伙伴已静默</p>
    <CompanionPageNote />
    <RouterView />
    <template #footer>
      <SiteFooter />
    </template>
  </AppShell>
  <CompanionDock />
</template>

<style scoped>
.companion-focus-chip {
  display: inline-flex;
  align-items: center;
  padding: 2px 10px;
  margin: 0 0 12px;
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  letter-spacing: 0.12em;
  color: var(--kapoj-text-secondary);
  border: 1px solid var(--kapoj-border-strong);
  border-radius: 999px;
}
</style>
