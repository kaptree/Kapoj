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

  const items: AdminNavGroup[] = [{ key: 'overview', label: '概览', items: [overviewItem] }]
  const userPermissionItems: AdminNavItem[] = []
  if (auth.can(PERM.user.read)) userPermissionItems.push(usersItem)
  if (auth.can(PERM.role.read)) userPermissionItems.push(rolesItem)
  if (userPermissionItems.length > 0) {
    items.push({ key: 'users', label: '用户与权限', items: userPermissionItems })
  }
  if (auth.canAny(GAMIFICATION_MANAGE_PERMISSIONS)) {
    items.push({ key: 'gamification', label: '游戏化', items: [gamificationItem] })
  }
  if (contentItems.length > 0) {
    items.push({ key: 'content', label: '内容', items: contentItems })
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
  gap: var(--kapoj-space-5);
  min-width: 0;
}

.admin-nav__group {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
}

.admin-nav__label {
  margin: 0 0 var(--kapoj-space-1);
  padding: 0 var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.admin-nav__item {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  min-height: 36px;
  padding: 0 var(--kapoj-space-2);
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

.admin-nav__item.is-active {
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
  border-color: color-mix(in srgb, var(--kapoj-primary) 24%, transparent);
}

.admin-nav__item.is-active::before {
  position: absolute;
  top: 8px;
  bottom: 8px;
  left: -1px;
  width: 2px;
  content: '';
  background: var(--kapoj-primary);
  border-radius: var(--kapoj-radius-xs);
  box-shadow: 0 0 8px var(--kapoj-primary);
}

.admin-nav__item--back {
  margin-top: var(--kapoj-space-1);
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
  border-radius: 0;
}

.admin-nav__icon {
  flex: none;
  width: 16px;
  font-size: 14px;
  text-align: center;
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
