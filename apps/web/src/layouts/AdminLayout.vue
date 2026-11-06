<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'

import AdminNav from '@/components/layout/AdminNav.vue'
import UserMenu from '@/components/UserMenu.vue'
import AppThemeToggle from '@/components/ui/AppThemeToggle.vue'

const route = useRoute()
const menuOpen = ref(false)
const menuButton = ref<{ $el?: HTMLElement } | null>(null)

const pageTitle = computed(() => route.meta.title ?? '管理后台')

function focusMenuButton() {
  const element = menuButton.value?.$el
  if (element instanceof HTMLElement && element.offsetParent !== null) element.focus()
}

function openMenu() {
  menuOpen.value = true
}

function closeMenu(restoreFocus = true) {
  if (!menuOpen.value) return
  menuOpen.value = false
  if (restoreFocus) void nextTick(focusMenuButton)
}

function onDrawerHide() {
  void nextTick(focusMenuButton)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && menuOpen.value) {
    event.preventDefault()
    closeMenu()
  }
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))

watch(
  () => route.fullPath,
  () => closeMenu(false),
)
</script>

<template>
  <div class="admin-shell">
    <aside class="admin-sidebar">
      <RouterLink to="/" class="admin-brand" aria-label="kapOJ 首页">
        <span class="admin-brand__mark" aria-hidden="true">K</span>
        <span class="admin-brand__name">kap<strong>OJ</strong></span>
        <span class="admin-brand__badge">管理后台</span>
      </RouterLink>
      <AdminNav id="admin-sidebar-nav" />
      <div class="admin-sidebar__footer">
        <span class="admin-sidebar__pulse" aria-hidden="true" />
        <span>管理控制台</span>
        <span class="admin-sidebar__version">M4-C</span>
      </div>
    </aside>

    <div class="admin-frame" :inert="menuOpen" :aria-hidden="menuOpen ? 'true' : undefined">
      <header class="admin-header">
        <Button
          ref="menuButton"
          class="admin-menu-toggle"
          icon="pi pi-bars"
          text
          size="small"
          aria-label="打开导航"
          aria-haspopup="dialog"
          :aria-expanded="menuOpen"
          @click="openMenu"
        />
        <RouterLink to="/" class="admin-header__brand" aria-label="kapOJ 首页">
          kap<strong>OJ</strong>
        </RouterLink>
        <div class="admin-header__context">
          <span class="admin-header__kicker">KAPOJ // ADMIN</span>
          <span class="admin-header__title">{{ pageTitle }}</span>
        </div>
        <div class="admin-header__actions">
          <AppThemeToggle />
          <UserMenu />
        </div>
      </header>
      <main class="admin-main">
        <RouterView />
      </main>
    </div>

    <Drawer
      v-model:visible="menuOpen"
      position="left"
      class="admin-drawer"
      :style="{ width: 'min(88vw, 300px)' }"
      :dismissable="true"
      :modal="true"
      :show-close-icon="true"
      :close-button-props="{ 'aria-label': '关闭导航' }"
      @hide="onDrawerHide"
    >
      <template #header>
        <RouterLink
          to="/"
          class="admin-brand admin-brand--drawer"
          aria-label="kapOJ 首页"
          @click="closeMenu()"
        >
          <span class="admin-brand__mark" aria-hidden="true">K</span>
          <span class="admin-brand__name">kap<strong>OJ</strong></span>
          <span class="admin-brand__badge">管理后台</span>
        </RouterLink>
      </template>
      <div class="admin-drawer__nav">
        <AdminNav id="admin-mobile-nav" @navigate="closeMenu()" />
      </div>
    </Drawer>
  </div>
</template>

<style scoped>
.admin-shell {
  position: relative;
  display: grid;
  grid-template-columns: var(--kapoj-sidebar-width) minmax(0, 1fr);
  min-height: 100vh;
  background: var(--kapoj-bg);
  isolation: isolate;
}

.admin-shell::before {
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

.admin-sidebar {
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

.admin-brand {
  display: flex;
  flex: none;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding: 0 var(--kapoj-space-1);
  color: var(--kapoj-text);
  text-decoration: none;
}

.admin-brand:hover {
  text-decoration: none;
}

.admin-brand__mark {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  color: var(--kapoj-bg);
  font-family: var(--kapoj-font-mono);
  font-size: 15px;
  font-weight: 700;
  background: var(--kapoj-primary);
  border-radius: var(--kapoj-radius-sm);
  box-shadow: var(--kapoj-glow-primary);
}

.admin-brand__name {
  font-family: var(--kapoj-font-mono);
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.04em;
}

.admin-brand__name strong {
  color: var(--kapoj-primary);
  font-weight: inherit;
}

.admin-brand__badge {
  padding: 1px var(--kapoj-space-1);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  letter-spacing: 0.08em;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}

.admin-sidebar__footer {
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

.admin-sidebar__pulse {
  width: 6px;
  height: 6px;
  background: var(--kapoj-success);
  border-radius: 50%;
  box-shadow: 0 0 8px var(--kapoj-success);
}

.admin-sidebar__version {
  margin-left: auto;
  color: var(--kapoj-text-secondary);
}

.admin-frame {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 100vh;
}

.admin-header {
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

.admin-header__context {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.admin-header__kicker {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  letter-spacing: 0.16em;
}

.admin-header__title {
  overflow: hidden;
  color: var(--kapoj-text);
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.admin-header__actions {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-1);
  margin-left: auto;
}

.admin-menu-toggle {
  display: none;
  flex: none;
  width: 2rem;
  height: 2rem;
  padding: 0;
  color: var(--kapoj-text-secondary);
  border-radius: var(--kapoj-radius-sm);
}

.admin-menu-toggle:hover {
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.admin-header__brand {
  display: none;
  align-items: center;
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 16px;
  font-weight: 700;
  letter-spacing: -0.04em;
  text-decoration: none;
}

.admin-header__brand strong {
  color: var(--kapoj-primary);
  font-weight: inherit;
}

.admin-main {
  flex: 1;
  width: 100%;
  max-width: var(--kapoj-content-max);
  padding: var(--kapoj-space-5);
  margin: 0 auto;
}

.admin-drawer__nav {
  padding: var(--kapoj-space-1) 0;
}

@media (max-width: 900px) {
  .admin-shell {
    grid-template-columns: minmax(0, 1fr);
  }

  .admin-sidebar {
    display: none;
  }

  .admin-menu-toggle,
  .admin-header__brand {
    display: inline-flex;
  }

  .admin-header__context {
    display: none;
  }

  .admin-header {
    gap: var(--kapoj-space-2);
    padding: 0 var(--kapoj-space-3);
  }

  .admin-main {
    padding: var(--kapoj-space-4) var(--kapoj-space-3);
  }
}

@media (max-width: 600px) {
  .admin-main {
    padding: var(--kapoj-space-3) var(--kapoj-space-2);
  }
}

@media (prefers-reduced-motion: reduce) {
  .admin-header {
    backdrop-filter: none;
  }
}
</style>
