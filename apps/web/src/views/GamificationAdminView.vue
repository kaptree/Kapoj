<script setup lang="ts">
import { computed, ref } from 'vue'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'

import AchievementAdminPanel from '@/components/admin/AchievementAdminPanel.vue'
import BadgeAdminPanel from '@/components/admin/BadgeAdminPanel.vue'
import TaskAdminPanel from '@/components/admin/TaskAdminPanel.vue'
import TitleAdminPanel from '@/components/admin/TitleAdminPanel.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { hasRole } from '@/router/guard'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const allowed = computed(() => hasRole(auth.user?.role ?? null, ['super_admin']))
const activeTab = ref('achievements')
</script>

<template>
  <div class="admin-page">
    <template v-if="!allowed">
      <AppPageHeader
        eyebrow="ADMIN // GAMIFICATION"
        title="游戏化管理"
        description="维护成就、任务、徽章与称号。"
      />
      <AppSurface padding="lg">
        <AppEmptyState
          icon="pi pi-lock"
          title="没有权限"
          description="仅超级管理员可以访问游戏化管理。"
        />
      </AppSurface>
    </template>

    <template v-else>
      <AppPageHeader
        eyebrow="ADMIN // GAMIFICATION"
        title="游戏化管理"
        description="维护成就、任务、徽章与称号。"
      />

      <AppSurface padding="lg" class="admin-surface">
        <Tabs v-model:value="activeTab" lazy>
          <TabList>
            <Tab value="achievements">成就</Tab>
            <Tab value="tasks">任务</Tab>
            <Tab value="badges">徽章</Tab>
            <Tab value="titles">称号</Tab>
          </TabList>
          <TabPanels>
            <TabPanel value="achievements">
              <AchievementAdminPanel />
            </TabPanel>
            <TabPanel value="tasks">
              <TaskAdminPanel />
            </TabPanel>
            <TabPanel value="badges">
              <BadgeAdminPanel />
            </TabPanel>
            <TabPanel value="titles">
              <TitleAdminPanel />
            </TabPanel>
          </TabPanels>
        </Tabs>
      </AppSurface>
    </template>
  </div>
</template>

<style scoped>
.admin-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.admin-surface {
  min-width: 0;
}
</style>
