<script setup lang="ts">
import { computed, ref } from 'vue'

import AchievementAdminPanel from '@/components/admin/AchievementAdminPanel.vue'
import BadgeAdminPanel from '@/components/admin/BadgeAdminPanel.vue'
import TaskAdminPanel from '@/components/admin/TaskAdminPanel.vue'
import TitleAdminPanel from '@/components/admin/TitleAdminPanel.vue'
import { hasRole } from '@/router/guard'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const allowed = computed(() => hasRole(auth.user?.role ?? null, ['super_admin']))
const activeTab = ref('achievements')
</script>

<template>
  <el-result
    v-if="!allowed"
    icon="warning"
    title="没有权限"
    sub-title="仅超级管理员可以访问游戏化管理"
  />

  <el-card v-else shadow="never">
    <template #header>
      <div class="head">
        <div>
          <span class="page-title">游戏化管理</span>
          <span class="sub">维护成就、任务、徽章与称号</span>
        </div>
      </div>
    </template>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="成就" name="achievements" lazy>
        <AchievementAdminPanel />
      </el-tab-pane>
      <el-tab-pane label="任务" name="tasks" lazy>
        <TaskAdminPanel />
      </el-tab-pane>
      <el-tab-pane label="徽章" name="badges" lazy>
        <BadgeAdminPanel />
      </el-tab-pane>
      <el-tab-pane label="称号" name="titles" lazy>
        <TitleAdminPanel />
      </el-tab-pane>
    </el-tabs>
  </el-card>
</template>

<style scoped>
.head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.page-title {
  font-size: 18px;
  font-weight: 600;
}
.sub {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
@media (max-width: 640px) {
  .sub {
    display: block;
    margin: 4px 0 0;
  }
}
</style>
