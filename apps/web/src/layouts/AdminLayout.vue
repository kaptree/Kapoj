<script setup lang="ts">
import AdminNav from '@/components/layout/AdminNav.vue'
import AppShell from '@/components/layout/AppShell.vue'
</script>

<template>
  <AppShell
    title-fallback="管理后台"
    kicker="KAPOJ // ADMIN"
    brand-badge="管理后台"
    sidebar-footer-text="管理控制台"
  >
    <template #nav="{ navigate }">
      <AdminNav @navigate="navigate" />
    </template>
    <RouterView />
  </AppShell>
</template>
