<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { getAdminStats } from '@/api/admin'
import type { AdminStats } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import {
  CONTEST_MANAGE_PERMISSIONS,
  GAMIFICATION_MANAGE_PERMISSIONS,
  PERM,
  PROBLEM_MANAGE_PERMISSIONS,
} from '@/permissions'
import { useAuthStore } from '@/stores/auth'

interface StatCard {
  key: string
  label: string
  value: number
  detail?: string
}

const auth = useAuthStore()

const canViewDashboard = computed(() => auth.can(PERM.admin.dashboard.view))
const canViewAudit = computed(() => auth.can(PERM.admin.audit.view))

const canGamification = computed(() => auth.canAny(GAMIFICATION_MANAGE_PERMISSIONS))
const canProblems = computed(() => auth.canAny(PROBLEM_MANAGE_PERMISSIONS))
const canContests = computed(() => auth.canAny(CONTEST_MANAGE_PERMISSIONS))
const canUsers = computed(() => auth.can(PERM.user.read))
const canRoles = computed(() => auth.can(PERM.role.read))

const stats = ref<AdminStats | null>(null)
const loading = ref(false)
const error = ref('')

const cards = computed<StatCard[]>(() => {
  const s = stats.value
  if (!s) return []
  return [
    {
      key: 'users',
      label: '用户',
      value: s.users,
      detail: `活跃 ${s.activeUsers} · 封禁 ${s.bannedUsers}`,
    },
    { key: 'problems', label: '题目', value: s.problems, detail: `公开 ${s.publicProblems}` },
    { key: 'contests', label: '比赛', value: s.contests },
    {
      key: 'submissions',
      label: '提交',
      value: s.submissions,
      detail: `今日 ${s.todaySubmissions}`,
    },
    { key: 'roles', label: '角色', value: s.roles },
  ]
})

const shortcuts = computed(() =>
  [
    {
      key: 'gamification',
      path: '/admin/gamification',
      label: '游戏化管理',
      icon: 'pi pi-shield',
      show: canGamification.value,
    },
    {
      key: 'problems',
      path: '/problems',
      label: '题库管理',
      icon: 'pi pi-book',
      show: canProblems.value,
    },
    {
      key: 'contests',
      path: '/contests',
      label: '比赛管理',
      icon: 'pi pi-trophy',
      show: canContests.value,
    },
  ].filter((item) => item.show),
)

/** Always-visible admin entry points (user/role pages), independent of dashboard access. */
const managementShortcuts = computed(() =>
  [
    {
      key: 'users',
      path: '/admin/users',
      label: '用户管理',
      icon: 'pi pi-users',
      show: canUsers.value,
    },
    {
      key: 'roles',
      path: '/admin/roles',
      label: '角色权限',
      icon: 'pi pi-key',
      show: canRoles.value,
    },
    {
      key: 'audit',
      path: '/admin/audit',
      label: '审计日志',
      icon: 'pi pi-history',
      show: canViewAudit.value,
    },
  ].filter((item) => item.show),
)

async function loadStats() {
  loading.value = true
  error.value = ''
  try {
    stats.value = await getAdminStats()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (canViewDashboard.value) void loadStats()
})
</script>

<template>
  <div class="admin-dashboard">
    <AppPageHeader
      eyebrow="ADMIN // OVERVIEW"
      title="管理后台"
      description="平台运营概览与管理入口。"
    />

    <AppSurface v-if="managementShortcuts.length > 0" padding="lg" class="admin-dashboard__entry">
      <h2 class="admin-dashboard__section-title">管理快捷入口</h2>
      <div class="admin-dashboard__shortcuts">
        <RouterLink
          v-for="item in managementShortcuts"
          :key="item.key"
          :to="item.path"
          class="admin-dashboard__shortcut"
        >
          <i :class="item.icon" aria-hidden="true" />
          <span>{{ item.label }}</span>
        </RouterLink>
      </div>
    </AppSurface>

    <template v-if="canViewDashboard">
      <AppLoadingState v-if="loading" label="正在加载仪表盘" />
      <AppErrorState v-else-if="error" :description="error" @retry="loadStats" />
      <template v-else-if="stats">
        <div class="admin-dashboard__grid">
          <AppSurface v-for="card in cards" :key="card.key" class="admin-dashboard__card">
            <span class="admin-dashboard__card-label">{{ card.label }}</span>
            <strong class="admin-dashboard__card-value num">{{ card.value }}</strong>
            <span v-if="card.detail" class="admin-dashboard__card-detail">{{ card.detail }}</span>
          </AppSurface>
        </div>

        <AppSurface v-if="canViewAudit" class="admin-dashboard__audit">
          <h2 class="admin-dashboard__section-title">最近审计</h2>
          <ul v-if="stats.recentAudit.length > 0" class="admin-dashboard__audit-list">
            <li
              v-for="item in stats.recentAudit"
              :key="item.id"
              class="admin-dashboard__audit-item"
            >
              <span class="admin-dashboard__audit-summary">{{ item.summary }}</span>
              <span class="admin-dashboard__audit-meta">
                {{ item.action }} ·
                {{ new Date(item.createdAt).toLocaleString('zh-CN') }}
              </span>
            </li>
          </ul>
          <AppEmptyState
            v-else
            icon="pi pi-clock"
            title="暂无审计记录"
            description="最近的管理操作会显示在这里。"
          />
        </AppSurface>
      </template>
    </template>

    <AppSurface v-else padding="lg" class="admin-dashboard__welcome">
      <div class="admin-dashboard__welcome-body">
        <h2 class="admin-dashboard__section-title">欢迎使用管理后台</h2>
        <p class="admin-dashboard__welcome-text">从下面的入口开始管理平台内容。</p>
      </div>
      <div v-if="shortcuts.length > 0" class="admin-dashboard__shortcuts">
        <RouterLink
          v-for="item in shortcuts"
          :key="item.key"
          :to="item.path"
          class="admin-dashboard__shortcut"
        >
          <i :class="item.icon" aria-hidden="true" />
          <span>{{ item.label }}</span>
        </RouterLink>
      </div>
      <AppEmptyState
        v-else
        icon="pi pi-lock"
        title="暂无可用入口"
        description="你还没有可用的管理功能入口。"
      />
    </AppSurface>
  </div>
</template>

<style scoped>
.admin-dashboard {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.admin-dashboard__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: var(--kapoj-space-3);
}

.admin-dashboard__card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  min-width: 0;
}

.admin-dashboard__card-label {
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.admin-dashboard__card-value {
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 1.75rem;
  font-weight: 700;
  letter-spacing: -0.02em;
}

.admin-dashboard__card-detail {
  color: var(--kapoj-text-muted);
  font-size: 0.75rem;
}

.admin-dashboard__audit {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.admin-dashboard__section-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.admin-dashboard__audit-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding: 0;
  margin: 0;
  list-style: none;
}

.admin-dashboard__audit-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--kapoj-space-2) 0;
  border-bottom: 1px solid var(--kapoj-border);
}

.admin-dashboard__audit-item:last-child {
  border-bottom: none;
}

.admin-dashboard__audit-summary {
  color: var(--kapoj-text);
  font-size: 0.875rem;
}

.admin-dashboard__audit-meta {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 0.75rem;
}

.admin-dashboard__welcome {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.admin-dashboard__entry {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.admin-dashboard__welcome-text {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.admin-dashboard__shortcuts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.admin-dashboard__shortcut {
  display: inline-flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-2) var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
  font-weight: 500;
  text-decoration: none;
  background: var(--kapoj-surface-hover);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
  transition:
    color var(--kapoj-transition),
    border-color var(--kapoj-transition);
}

.admin-dashboard__shortcut:hover {
  color: var(--kapoj-primary);
  text-decoration: none;
  border-color: var(--kapoj-primary);
}
</style>
