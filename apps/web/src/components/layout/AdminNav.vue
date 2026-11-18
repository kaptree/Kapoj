<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import {
  CONTEST_MANAGE_PERMISSIONS,
  GAMIFICATION_MANAGE_PERMISSIONS,
  PERM,
  PROBLEM_MANAGE_PERMISSIONS,
} from '@/permissions'
import { useAuthStore } from '@/stores/auth'

interface AdminNavItem {
  key: string
  path: string
  label: string
  icon: string
}

interface AdminNavGroup {
  key: string
  label: string
  items: AdminNavItem[]
}

const emit = defineEmits<{ navigate: [] }>()
const route = useRoute()
const auth = useAuthStore()

const overviewItem: AdminNavItem = {
  key: 'overview',
  path: '/admin',
  label: '概览',
  icon: 'pi pi-th-large',
}

const gamificationItem: AdminNavItem = {
  key: 'gamification',
  path: '/admin/gamification',
  label: '游戏化管理',
  icon: 'pi pi-shield',
}

const usersItem: AdminNavItem = {
  key: 'users',
  path: '/admin/users',
  label: '用户管理',
  icon: 'pi pi-users',
}

const rolesItem: AdminNavItem = {
  key: 'roles',
  path: '/admin/roles',
  label: '角色权限',
  icon: 'pi pi-key',
}

const auditItem: AdminNavItem = {
  key: 'audit',
  path: '/admin/audit',
  label: '审计日志',
  icon: 'pi pi-history',
}

const learningPathsItem: AdminNavItem = {
  key: 'learning-paths',
  path: '/admin/learning-paths',
  label: '学习路线',
  icon: 'pi pi-sitemap',
}

const ratingItem: AdminNavItem = {
  key: 'rating',
  path: '/admin/rating',
  label: 'Rating 设置',
  icon: 'pi pi-chart-line',
}

const aiItem: AdminNavItem = {
  key: 'ai',
  path: '/admin/ai',
  label: 'AI 设置',
  icon: 'pi pi-sparkles',
}

const agentsItem: AdminNavItem = {
  key: 'agents',
  path: '/admin/agents',
  label: 'Agent 管理',
  icon: 'pi pi-microchip-ai',
}

const siteItem: AdminNavItem = {
  key: 'site',
  path: '/admin/site',
  label: '站点设置',
  icon: 'pi pi-image',
}

const mailItem: AdminNavItem = {
  key: 'mail',
  path: '/admin/mail',
  label: '邮件设置',
  icon: 'pi pi-envelope',
}

const backupItem: AdminNavItem = {
  key: 'backups',
  path: '/admin/backups',
  label: '数据备份',
  icon: 'pi pi-database',
}

const jobsItem: AdminNavItem = {
  key: 'jobs',
  path: '/admin/jobs',
  label: '定时任务',
  icon: 'pi pi-clock',
}

const groups = computed<AdminNavGroup[]>(() => {
  const contentItems: AdminNavItem[] = []
  if (auth.canAny(PROBLEM_MANAGE_PERMISSIONS)) {
    contentItems.push({ key: 'problems', path: '/problems', label: '题库管理', icon: 'pi pi-book' })
  }
  if (auth.canAny(CONTEST_MANAGE_PERMISSIONS)) {
    contentItems.push({
      key: 'contests',
      path: '/contests',
      label: '比赛管理',
      icon: 'pi pi-trophy',
    })
  }
  if (auth.canAny([PERM.learningPath.official.manage, PERM.learningPath.admin.read])) {
    contentItems.push(learningPathsItem)
  }
  if (auth.can(PERM.certificate.manage)) {
    contentItems.push({
      key: 'certificates',
      path: '/admin/certificates',
      label: '证书审核',
      icon: 'pi pi-id-card',
    })
  }

  const items: AdminNavGroup[] = [{ key: 'overview', label: '概览', items: [overviewItem] }]
  const userPermissionItems: AdminNavItem[] = []
  if (auth.can(PERM.user.read)) userPermissionItems.push(usersItem)
  if (auth.can(PERM.role.read)) userPermissionItems.push(rolesItem)
  if (userPermissionItems.length > 0) {
    items.push({ key: 'users', label: '用户与权限', items: userPermissionItems })
  }
  if (auth.can(PERM.admin.audit.view)) {
    items.push({ key: 'audit', label: '审计', items: [auditItem] })
  }
  if (auth.can(PERM.rating.settings.manage)) {
    items.push({ key: 'rating', label: 'Rating', items: [ratingItem] })
  }
  if (auth.can(PERM.ai.settings.manage)) {
    items.push({ key: 'ai', label: 'AI', items: [aiItem] })
  }
  if (auth.can(PERM.agent.manage)) {
    items.push({ key: 'agents', label: 'Agent', items: [agentsItem] })
  }
  if (auth.canAny(GAMIFICATION_MANAGE_PERMISSIONS)) {
    items.push({ key: 'gamification', label: '游戏化', items: [gamificationItem] })
  }
  if (contentItems.length > 0) {
    items.push({ key: 'content', label: '内容', items: contentItems })
  }
  if (auth.can(PERM.site.settings.manage)) {
    items.push({ key: 'site', label: '站点', items: [siteItem] })
  }
  const systemItems: AdminNavItem[] = []
  if (auth.can(PERM.system.jobs.manage)) systemItems.push(jobsItem)
  if (auth.can(PERM.mail.settings.manage)) systemItems.push(mailItem)
  if (auth.can(PERM.system.backup.manage)) systemItems.push(backupItem)
  if (systemItems.length > 0) {
    items.push({ key: 'system', label: '系统', items: systemItems })
  }
  return items
})

function isActive(path: string): boolean {
  if (path === '/admin') return route.path === '/admin'
  if (path === '/') return route.path === '/'
  return route.path === path || route.path.startsWith(`${path}/`)
}
</script>

<template>
  <nav class="admin-nav" aria-label="管理导航">
    <div v-for="group in groups" :key="group.key" class="admin-nav__group">
      <p class="admin-nav__label">{{ group.label }}</p>
      <RouterLink
        v-for="item in group.items"
        :key="item.key"
        :to="item.path"
        class="admin-nav__item"
        :class="{ 'is-active': isActive(item.path) }"
        :data-nav="item.key"
        :aria-current="isActive(item.path) ? 'page' : undefined"
        @click="emit('navigate')"
      >
        <i class="admin-nav__icon" :class="item.icon" aria-hidden="true" />
        <span class="admin-nav__text">{{ item.label }}</span>
      </RouterLink>
    </div>

    <RouterLink
      to="/"
      class="admin-nav__item admin-nav__item--back"
      data-nav="back"
      @click="emit('navigate')"
    >
      <i class="admin-nav__icon pi pi-arrow-left" aria-hidden="true" />
      <span class="admin-nav__text">返回站点</span>
    </RouterLink>
  </nav>
</template>

<style scoped>
.admin-nav {
  display: flex;
  flex-direction: column;
  gap: 28px;
  min-width: 0;
}

.admin-nav__group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.admin-nav__label {
  margin: 0 0 6px;
  padding: 0 10px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.13em;
  text-transform: uppercase;
}

.admin-nav__item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 40px;
  padding: 0 var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 13px;
  font-weight: 500;
  text-decoration: none;
  border: 1px solid transparent;
  border-radius: var(--kapoj-radius-sm);
  transition:
    color var(--kapoj-transition),
    background var(--kapoj-transition),
    border-color var(--kapoj-transition);
}

.admin-nav__item:hover {
  color: var(--kapoj-text);
  text-decoration: none;
  background: var(--kapoj-surface-hover);
}

.admin-nav__item:focus-visible {
  outline: 2px solid var(--kapoj-border-focus);
  outline-offset: 1px;
}

.admin-nav__item.is-active {
  color: var(--kapoj-primary-strong);
  background: var(--kapoj-primary-soft);
  border-color: color-mix(in srgb, var(--kapoj-brand) 18%, transparent);
}

.admin-nav__item.is-active::before {
  position: absolute;
  top: 9px;
  bottom: 9px;
  left: 0;
  width: 3px;
  content: '';
  background: var(--kapoj-brand);
  border-radius: 0 999px 999px 0;
}

.admin-nav__item--back {
  min-height: 48px;
  padding-top: var(--kapoj-space-3);
  margin-top: var(--kapoj-space-1);
  border-top: 1px solid var(--kapoj-border);
  border-radius: 0;
}

.admin-nav__icon {
  flex: none;
  width: 18px;
  color: var(--kapoj-text-muted);
  font-size: 15px;
  text-align: center;
  transition: color var(--kapoj-transition);
}

.admin-nav__item:hover .admin-nav__icon,
.admin-nav__item.is-active .admin-nav__icon {
  color: var(--kapoj-primary-strong);
}

.admin-nav__text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (prefers-reduced-motion: reduce) {
  .admin-nav__item {
    transition: none;
  }
}
</style>
