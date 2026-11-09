<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'

import AppBrand from '@/components/layout/AppBrand.vue'
import NotificationBell from '@/components/layout/NotificationBell.vue'
import UserMenu from '@/components/UserMenu.vue'
import AppThemeToggle from '@/components/ui/AppThemeToggle.vue'
import { useLayoutDrawer, type LayoutMenuButtonHandle } from '@/composables/useLayoutDrawer'

const props = withDefaults(
  defineProps<{
    titleFallback?: string
    kicker?: string
    brandBadge?: string
    sidebarFooterText?: string
    sidebarVersion?: string
    showUserMenu?: boolean
  }>(),
  {
    titleFallback: '控制台',
    kicker: 'KAPOJ // CONSOLE',
    brandBadge: '',
    sidebarFooterText: '系统在线',
    sidebarVersion: 'M4-C',
    showUserMenu: true,
  },
)

const route = useRoute()
const menuButton = ref<LayoutMenuButtonHandle | null>(null)
const { menuOpen, openMenu, closeMenu, onDrawerHide } = useLayoutDrawer(menuButton)

const pageTitle = computed(() => route.meta.title ?? props.titleFallback)
</script>

<template>
  <div class="app-shell">
    <aside class="app-sidebar">
      <AppBrand :badge="brandBadge" />
      <div id="app-sidebar-nav" class="app-sidebar__nav">
        <slot name="nav" :navigate="closeMenu" />
      </div>
      <div class="app-sidebar__footer">
        <span class="app-sidebar__pulse" aria-hidden="true" />
        <span>{{ sidebarFooterText }}</span>
        <span class="app-sidebar__version">{{ sidebarVersion }}</span>
      </div>
    </aside>

    <div class="app-frame" :inert="menuOpen" :aria-hidden="menuOpen ? 'true' : undefined">
      <header class="app-header">
        <Button
          ref="menuButton"
          class="app-menu-toggle"
          icon="pi pi-bars"
          text
          size="small"
          aria-label="打开导航"
          aria-haspopup="dialog"
          :aria-expanded="menuOpen"
          @click="openMenu"
        />
        <AppBrand class="app-header__brand" />
        <div class="app-header__context">
          <span class="app-header__kicker">{{ kicker }}</span>
          <span class="app-header__title">{{ pageTitle }}</span>
        </div>
        <div class="app-header__actions">
          <AppThemeToggle />
          <NotificationBell v-if="showUserMenu" />
          <UserMenu v-if="showUserMenu" />
        </div>
      </header>
      <main class="app-main">
        <slot />
      </main>
      <footer v-if="$slots.footer" class="app-footer">
        <slot name="footer" />
      </footer>
    </div>

    <Drawer
      v-model:visible="menuOpen"
      position="left"
      class="app-drawer"
      :style="{ width: 'min(88vw, 300px)' }"
      :dismissable="true"
      :modal="true"
      :show-close-icon="true"
      :close-button-props="{ 'aria-label': '关闭导航' }"
      @hide="onDrawerHide"
    >
      <template #header>
        <AppBrand :badge="brandBadge" drawer @click="closeMenu()" />
      </template>
      <div id="app-mobile-nav" class="app-drawer__nav">
        <slot name="nav" :navigate="closeMenu" />
      </div>
    </Drawer>
  </div>
</template>

<style scoped>
.app-shell {
  position: relative;
  display: grid;
  grid-template-columns: var(--kapoj-sidebar-width) minmax(0, 1fr);
  min-height: 100vh;
  background: var(--kapoj-bg);
  isolation: isolate;
}

.app-shell::before {
  position: fixed;
  inset: 0;
  z-index: -1;
  pointer-events: none;
  content: '';
  background-image:
    linear-gradient(var(--kapoj-grid-line) 1px, transparent 1px),
    linear-gradient(90deg, var(--kapoj-grid-line) 1px, transparent 1px);
  background-size: 56px 56px;
  -webkit-mask-image: radial-gradient(ellipse at 68% 0%, black 0%, transparent 68%);
  mask-image: radial-gradient(ellipse at 68% 0%, black 0%, transparent 68%);
}

.app-sidebar {
  position: sticky;
  top: 0;
  z-index: 40;
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-6);
  height: 100vh;
  padding: var(--kapoj-space-5) var(--kapoj-space-4);
  overflow-y: auto;
  background: var(--kapoj-bg-elevated);
  border-right: 1px solid var(--kapoj-border);
}

.app-sidebar__nav {
  min-width: 0;
  min-height: 0;
}

.app-sidebar__footer {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  margin-top: auto;
  padding: var(--kapoj-space-3) var(--kapoj-space-2) 0;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  letter-spacing: 0.08em;
  border-top: 1px solid var(--kapoj-border);
}

.app-sidebar__pulse {
  width: 6px;
  height: 6px;
  background: var(--kapoj-success);
  border-radius: 50%;
  box-shadow: 0 0 8px var(--kapoj-success);
}

.app-sidebar__version {
  margin-left: auto;
  color: var(--kapoj-text-secondary);
}

.app-frame {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 100vh;
}

.app-header {
  position: sticky;
  top: 0;
  z-index: 30;
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-4);
  height: var(--kapoj-header-height);
  padding: 0 var(--kapoj-space-5);
  background: color-mix(in srgb, var(--kapoj-bg) 86%, transparent);
  border-bottom: 1px solid var(--kapoj-border);
  -webkit-backdrop-filter: blur(14px);
  backdrop-filter: blur(14px);
}

.app-header__context {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.app-header__kicker {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  letter-spacing: 0.16em;
}

.app-header__title {
  overflow: hidden;
  color: var(--kapoj-text);
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.app-header__actions {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-1);
  margin-left: auto;
}

.app-menu-toggle {
  display: none;
  flex: none;
  width: 2rem;
  height: 2rem;
  padding: 0;
  color: var(--kapoj-text-secondary);
  border-radius: var(--kapoj-radius-sm);
}

.app-menu-toggle:hover {
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.app-header__brand.app-brand {
  display: none;
}

.app-main {
  flex: 1;
  width: 100%;
  max-width: var(--kapoj-content-max);
  padding: var(--kapoj-space-5);
  margin: 0 auto;
}

.app-footer {
  width: 100%;
  max-width: var(--kapoj-content-max);
  padding: 0 var(--kapoj-space-5) var(--kapoj-space-5);
  margin: 0 auto;
}

.app-drawer__nav {
  padding: var(--kapoj-space-1) 0;
}

@media (max-width: 900px) {
  .app-shell {
    grid-template-columns: minmax(0, 1fr);
  }

  .app-sidebar {
    display: none;
  }

  .app-menu-toggle,
  .app-header__brand.app-brand {
    display: inline-flex;
  }

  .app-header__context {
    display: none;
  }

  .app-header {
    gap: var(--kapoj-space-2);
    padding: 0 var(--kapoj-space-3);
  }

  .app-main {
    padding: var(--kapoj-space-4) var(--kapoj-space-3);
  }

  .app-footer {
    padding: 0 var(--kapoj-space-3) var(--kapoj-space-4);
  }
}

@media (max-width: 600px) {
  .app-main {
    padding: var(--kapoj-space-3) var(--kapoj-space-2);
  }

  .app-footer {
    padding: 0 var(--kapoj-space-2) var(--kapoj-space-3);
  }
}

@media (prefers-reduced-motion: reduce) {
  .app-header {
    backdrop-filter: none;
  }
}
</style>
