<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Component } from 'vue'
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
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'

interface AdminTab {
  value: string
  label: string
  permission: string
  panel: Component
}

const auth = useAuthStore()

const allTabs: AdminTab[] = [
  {
    value: 'achievements',
    label: '成就',
    permission: PERM.gamification.achievement.manage,
    panel: AchievementAdminPanel,
  },
  {
    value: 'tasks',
    label: '任务',
    permission: PERM.gamification.task.manage,
    panel: TaskAdminPanel,
  },
  {
    value: 'badges',
    label: '徽章',
    permission: PERM.gamification.badge.manage,
    panel: BadgeAdminPanel,
  },
  {
    value: 'titles',
    label: '称号',
    permission: PERM.gamification.title.manage,
    panel: TitleAdminPanel,
  },
]

const tabs = computed(() => allTabs.filter((tab) => auth.can(tab.permission)))
const allowed = computed(() => tabs.value.length > 0)
const activeTab = ref('')

watch(
  tabs,
  (list) => {
    if (!list.some((tab) => tab.value === activeTab.value)) {
      activeTab.value = list[0]?.value ?? ''
    }
  },
  { immediate: true },
)
</script>

<template>
  <div class="admin-page">
    <AppPageHeader
      eyebrow="ADMIN // GAMIFICATION"
      title="游戏化管理"
      description="维护成就、任务、徽章与称号。"
    />

    <AppSurface v-if="!allowed" padding="lg">
      <AppEmptyState
        icon="pi pi-lock"
        title="没有权限"
        description="你没有游戏化管理权限，无法访问此页面。"
      />
    </AppSurface>

    <AppSurface v-else padding="lg" class="admin-surface">
      <Tabs v-model:value="activeTab" lazy>
        <TabList>
          <Tab v-for="tab in tabs" :key="tab.value" :value="tab.value">{{ tab.label }}</Tab>
        </TabList>
        <TabPanels>
          <TabPanel v-for="tab in tabs" :key="tab.value" :value="tab.value">
            <component :is="tab.panel" />
          </TabPanel>
        </TabPanels>
      </Tabs>
    </AppSurface>
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
