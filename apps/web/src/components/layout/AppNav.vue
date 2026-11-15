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
      items: auth.isLoggedIn ? growthItems : guestGrowthItems,
    })
  }
  if (auth.isLoggedIn && showAdminEntry.value) {
    list.push({ key: 'admin', label: '管理', items: [adminItem] })
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
    <div
      v-for="group in groups"
      :key="group.key"
      class="app-nav__group"
      :class="{ 'app-nav__group--admin': group.key === 'admin' }"
    >
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
  gap: 28px;
  min-width: 0;
}

.app-nav__group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.app-nav__group--admin {
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.app-nav__label {
  margin: 0 0 6px;
  padding: 0 10px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.13em;
  text-transform: uppercase;
}

.app-nav__item {
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

.app-nav__item:hover {
  color: var(--kapoj-text);
  text-decoration: none;
  background: var(--kapoj-surface-hover);
}

.app-nav__item:focus-visible {
  outline: 2px solid var(--kapoj-border-focus);
  outline-offset: 1px;
}

.app-nav__item.is-active {
  color: var(--kapoj-primary-strong);
  background: var(--kapoj-primary-soft);
  border-color: color-mix(in srgb, var(--kapoj-brand) 18%, transparent);
}

.app-nav__item.is-active::before {
  position: absolute;
  top: 9px;
  bottom: 9px;
  left: 0;
  width: 3px;
  content: '';
  background: var(--kapoj-brand);
  border-radius: 0 999px 999px 0;
}

.app-nav__item .app-nav__icon {
  flex: none;
  width: 18px;
  color: var(--kapoj-text-muted);
  font-size: 15px;
  text-align: center;
  transition: color var(--kapoj-transition);
}

.app-nav__item:hover .app-nav__icon {
  color: var(--kapoj-primary);
}

.app-nav__item.is-active .app-nav__icon {
  color: var(--kapoj-primary-strong);
}

.app-nav__item[data-nav='admin']:not(.is-active) .app-nav__icon {
  color: var(--kapoj-special);
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
