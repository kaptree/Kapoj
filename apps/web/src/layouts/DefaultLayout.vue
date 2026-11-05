<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import UserMenu from '@/components/UserMenu.vue'
import { hasRole } from '@/router/guard'
import { useAuthStore } from '@/stores/auth'

interface NavItem {
  path: string
  label: string
  // Milestone in which the module ships; items without one are available now.
  milestone?: string
}

const navItems: NavItem[] = [
  { path: '/', label: '首页' },
  { path: '/problems', label: '题库' },
  { path: '/submissions', label: '测评记录' },
  { path: '/contests', label: '比赛' },
  { path: '/leaderboard', label: '排行榜', milestone: 'M4' },
]

const growthItems: NavItem[] = [
  { path: '/achievements', label: '成就' },
  { path: '/tasks', label: '任务' },
  { path: '/shop/badges', label: '徽章商城' },
  { path: '/shop/titles', label: '称号商店' },
  { path: '/appearance', label: '装扮管理' },
]

const route = useRoute()
const auth = useAuthStore()
const active = computed(() => route.path)
const showAdminEntry = computed(() => hasRole(auth.user?.role ?? null, ['super_admin']))

// Narrow screens use an overlay drawer instead of a permanently visible sidebar.
const narrow = ref(false)
const menuOpen = ref(false)
const toggleButton = ref<{ $el?: HTMLElement } | null>(null)
const closeButton = ref<HTMLButtonElement | null>(null)
const toggleLabel = computed(() => (menuOpen.value ? '关闭导航' : '打开导航'))
const drawerOpen = computed(() => narrow.value && menuOpen.value)
let media: MediaQueryList | null = null

function focusToggle() {
  toggleButton.value?.$el?.focus()
}

function openMenu() {
  menuOpen.value = true
  void nextTick(() => closeButton.value?.focus())
}

function closeMenu(restoreFocus = true) {
  if (!menuOpen.value) return
  menuOpen.value = false
  if (restoreFocus && narrow.value) void nextTick(focusToggle)
}

function toggleMenu() {
  if (menuOpen.value) closeMenu()
  else openMenu()
}

function syncNarrow(query: MediaQueryList | MediaQueryListEvent) {
  narrow.value = query.matches
  if (!query.matches) closeMenu(false)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && drawerOpen.value) {
    event.preventDefault()
    closeMenu()
  }
}

onMounted(() => {
  media = window.matchMedia('(max-width: 768px)')
  syncNarrow(media)
  media.addEventListener('change', syncNarrow)
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  media?.removeEventListener('change', syncNarrow)
  window.removeEventListener('keydown', onKeydown)
})

watch(
  () => route.fullPath,
  () => closeMenu(),
)
</script>

<template>
  <el-container class="layout">
    <el-header class="header">
      <el-button
        v-if="narrow"
        ref="toggleButton"
        text
        class="menu-toggle"
        :aria-expanded="menuOpen"
        :aria-label="toggleLabel"
        aria-controls="app-nav"
        @click="toggleMenu"
      >
        导航
      </el-button>
      <RouterLink to="/" class="brand">kapOJ</RouterLink>
      <div class="header-right">
        <UserMenu />
      </div>
    </el-header>
    <el-container>
      <div v-if="drawerOpen" class="backdrop" @click="closeMenu()" />
      <el-aside
        id="app-nav"
        width="200px"
        class="aside"
        :class="{ open: drawerOpen }"
        :inert="narrow && !menuOpen"
        :aria-hidden="narrow && !menuOpen ? 'true' : undefined"
      >
        <div v-if="narrow" class="aside-head">
          <span class="aside-title">导航</span>
          <button
            ref="closeButton"
            type="button"
            class="aside-close"
            aria-label="关闭导航"
            @click="closeMenu()"
          >
            关闭
          </button>
        </div>
        <el-menu
          :default-active="active"
          :default-openeds="['growth']"
          router
          @select="closeMenu()"
        >
          <el-menu-item
            v-for="item in navItems"
            :key="item.path"
            :index="item.path"
            :disabled="Boolean(item.milestone)"
          >
            <span>{{ item.label }}</span>
            <span v-if="item.milestone" class="soon">{{ item.milestone }} 上线</span>
          </el-menu-item>
          <el-sub-menu index="growth">
            <template #title>成长</template>
            <el-menu-item v-for="item in growthItems" :key="item.path" :index="item.path">
              {{ item.label }}
            </el-menu-item>
            <el-menu-item v-if="showAdminEntry" index="/admin/gamification">
              游戏化管理
            </el-menu-item>
          </el-sub-menu>
        </el-menu>
      </el-aside>
      <el-main :inert="drawerOpen" :aria-hidden="drawerOpen ? 'true' : undefined">
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
  gap: 12px;
  border-bottom: 1px solid var(--el-border-color-light);
  background: var(--el-bg-color);
}
.brand {
  font-size: 20px;
  font-weight: 700;
  color: var(--el-color-primary);
  text-decoration: none;
}
.header-right {
  display: flex;
  align-items: center;
  min-width: 0;
  margin-left: auto;
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
.menu-toggle {
  flex: none;
}
@media (max-width: 768px) {
  .header {
    gap: 8px;
    padding: 0 12px;
  }
  .brand {
    font-size: 18px;
  }
  .aside {
    position: fixed;
    top: 0;
    bottom: 0;
    left: 0;
    z-index: 2100;
    width: 240px !important;
    overflow-y: auto;
    transform: translateX(-100%);
    transition: transform 0.2s ease;
    box-shadow: 2px 0 12px rgb(0 0 0 / 18%);
  }
  .aside.open {
    transform: translateX(0);
  }
  .aside-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px 4px;
  }
  .aside-title {
    font-weight: 600;
  }
  .aside-close {
    padding: 4px 8px;
    color: var(--el-color-primary);
    font: inherit;
    background: transparent;
    border: 0;
    border-radius: 4px;
    cursor: pointer;
  }
  .aside-close:hover {
    background: var(--el-fill-color-light);
  }
  .aside-close:focus-visible {
    outline: 2px solid var(--el-color-primary);
    outline-offset: 2px;
  }
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 2050;
    background: rgb(0 0 0 / 35%);
  }
}
</style>
