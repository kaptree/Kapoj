<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

interface NavItem {
  path: string
  label: string
  // Milestone in which the module ships; items without one are available now.
  milestone?: string
}

const navItems: NavItem[] = [
  { path: '/', label: '首页' },
  { path: '/problems', label: '题库', milestone: 'M1' },
  { path: '/submissions', label: '测评记录', milestone: 'M1' },
  { path: '/contests', label: '比赛', milestone: 'M2' },
  { path: '/leaderboard', label: '排行榜', milestone: 'M4' },
]

const route = useRoute()
const active = computed(() => route.path)
</script>

<template>
  <el-container class="layout">
    <el-header class="header">
      <RouterLink to="/" class="brand">kapOJ</RouterLink>
      <div class="header-right">
        <slot name="user-menu" />
      </div>
    </el-header>
    <el-container>
      <el-aside width="200px" class="aside">
        <el-menu :default-active="active" router>
          <el-menu-item
            v-for="item in navItems"
            :key="item.path"
            :index="item.path"
            :disabled="Boolean(item.milestone)"
          >
            <span>{{ item.label }}</span>
            <span v-if="item.milestone" class="soon">{{ item.milestone }} 上线</span>
          </el-menu-item>
        </el-menu>
      </el-aside>
      <el-main>
        <RouterView />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.layout {
  min-height: 100%;
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--el-border-color-light);
  background: var(--el-bg-color);
}
.brand {
  font-size: 20px;
  font-weight: 700;
  color: var(--el-color-primary);
  text-decoration: none;
}
.aside {
  background: var(--el-bg-color);
  border-right: 1px solid var(--el-border-color-light);
}
.aside :deep(.el-menu) {
  border-right: none;
}
.soon {
  margin-left: auto;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
