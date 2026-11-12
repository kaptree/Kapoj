<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import { ADMIN_ACCESS_PERMISSIONS, GUEST_PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { useGuestStore } from '@/stores/guest'

interface NavItem {
  key: string
  path: string
  label: string
  icon: string
  milestone?: string
}

interface NavGroup {
  key: string
  label: string
  items: NavItem[]
}

const emit = defineEmits<{ navigate: [] }>()
const route = useRoute()
const auth = useAuthStore()
const guest = useGuestStore()

const primaryItems: NavItem[] = [
  { key: 'home', path: '/', label: '首页', icon: 'pi pi-home' },
  { key: 'problems', path: '/problems', label: '题库', icon: 'pi pi-book' },
  { key: 'submissions', path: '/submissions', label: '测评记录', icon: 'pi pi-list-check' },
  { key: 'contests', path: '/contests', label: '比赛', icon: 'pi pi-trophy' },
  { key: 'learning-paths', path: '/learning-paths', label: '学习路线', icon: 'pi pi-sitemap' },
  { key: 'leaderboard', path: '/leaderboard', label: '排行榜', icon: 'pi pi-chart-line' },
]

const growthItems: NavItem[] = [
  { key: 'achievements', path: '/achievements', label: '成就', icon: 'pi pi-star' },
  { key: 'tasks', path: '/tasks', label: '任务', icon: 'pi pi-check-square' },
  { key: 'badge-shop', path: '/shop/badges', label: '徽章商城', icon: 'pi pi-shop' },
  { key: 'title-shop', path: '/shop/titles', label: '称号商店', icon: 'pi pi-crown' },
  { key: 'appearance', path: '/appearance', label: '装扮管理', icon: 'pi pi-palette' },
]

const adminItem: NavItem = {
  key: 'admin',
  path: '/admin',
  label: '管理后台',
  icon: 'pi pi-shield',
}

const showAdminEntry = computed(() => auth.canAny(ADMIN_ACCESS_PERMISSIONS))
const growthVisible = computed(() => auth.isLoggedIn || guest.can(GUEST_PERM.growth))
const guestGrowthItems = growthItems.filter((item) => item.key !== 'appearance')

const groups = computed<NavGroup[]>(() => {
  const list: NavGroup[] = [{ key: 'core', label: '核心', items: primaryItems }]
  if (growthVisible.value) {
    list.push({
      key: 'growth',
      label: '成长',
      items: auth.isLoggedIn
        ? showAdminEntry.value
          ? [...growthItems, adminItem]
          : growthItems
        : guestGrowthItems,
    })
  }
  return list
})

function isActive(path: string): boolean {
  if (path === '/') return route.path === '/'
  return route.path === path || route.path.startsWith(`${path}/`)
}
</script>

<template>
  <nav class="app-nav" aria-label="主导航">
    <div v-for="group in groups" :key="group.key" class="app-nav__group">
      <p class="app-nav__label">{{ group.label }}</p>
      <template v-for="item in group.items" :key="item.key">
        <RouterLink
          v-if="!item.milestone"
          :to="item.path"
          class="app-nav__item"
          :class="{ 'is-active': isActive(item.path) }"
          :data-nav="item.key"
          :aria-current="isActive(item.path) ? 'page' : undefined"
          @click="emit('navigate')"
        >
          <i class="app-nav__icon" :class="item.icon" aria-hidden="true" />
          <span class="app-nav__text">{{ item.label }}</span>
        </RouterLink>
        <span
          v-else
          class="app-nav__item app-nav__item--disabled"
          :data-nav="item.key"
          aria-disabled="true"
          :title="`${item.milestone} 上线`"
        >
          <i class="app-nav__icon" :class="item.icon" aria-hidden="true" />
          <span class="app-nav__text">{{ item.label }}</span>
          <span class="app-nav__soon">{{ item.milestone }} 上线</span>
        </span>
      </template>
    </div>
  </nav>
</template>

<style scoped>
.app-nav {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  min-width: 0;
}

.app-nav__group {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
}

.app-nav__label {
  margin: 0 0 var(--kapoj-space-1);
  padding: 0 var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.app-nav__item {
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

.app-nav__item:hover {
  color: var(--kapoj-text);
  text-decoration: none;
  background: var(--kapoj-surface-hover);
}

.app-nav__item.is-active {
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
  border-color: color-mix(in srgb, var(--kapoj-primary) 24%, transparent);
}

.app-nav__item.is-active::before {
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

.app-nav__icon {
  flex: none;
  width: 16px;
  font-size: 14px;
  text-align: center;
}

.app-nav__text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.app-nav__soon {
  flex: none;
  padding: 1px var(--kapoj-space-1);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}

.app-nav__item--disabled,
.app-nav__item--disabled:hover {
  color: var(--kapoj-text-muted);
  cursor: not-allowed;
  background: transparent;
  border-color: transparent;
  opacity: 0.55;
}

@media (prefers-reduced-motion: reduce) {
  .app-nav__item {
    transition: none;
  }
}
</style>
