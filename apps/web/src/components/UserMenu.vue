<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import type { MenuItem } from 'primevue/menuitem'

import { useAuthStore } from '@/stores/auth'
import { ADMIN_ACCESS_PERMISSIONS } from '@/permissions'
import { formatCoins } from '@/utils/gamification'

interface MenuHandle {
  toggle: (event: Event) => void
}

const auth = useAuthStore()
const router = useRouter()
const menu = ref<MenuHandle | null>(null)
const menuOpen = ref(false)

const menuItems = computed<MenuItem[]>(() => {
  const user = auth.user
  if (!user) return []

  const items: MenuItem[] = [
    {
      label: '我的主页',
      icon: 'pi pi-user',
      command: () => {
        void router.push({ name: 'user-profile', params: { id: user.id } })
      },
    },
    {
      label: '装扮管理',
      icon: 'pi pi-palette',
      command: () => {
        void router.push({ name: 'appearance' })
      },
    },
    {
      label: '编辑资料',
      icon: 'pi pi-user-edit',
      command: () => {
        void router.push({ name: 'profile-edit' })
      },
    },
    {
      label: 'AI 设置',
      icon: 'pi pi-sparkles',
      command: () => router.push({ name: 'ai-settings' }),
    },
  ]

  if (auth.canAny(ADMIN_ACCESS_PERMISSIONS)) {
    items.push({
      label: '管理后台',
      icon: 'pi pi-shield',
      command: () => {
        void router.push({ name: 'admin-dashboard' })
      },
    })
  }

  items.push(
    { separator: true },
    {
      label: '退出登录',
      icon: 'pi pi-sign-out',
      command: () => {
        void logout()
      },
    },
  )

  return items
})

function toggleMenu(event: Event) {
  menu.value?.toggle(event)
}

function onMenuShow() {
  menuOpen.value = true
  // Keep the coin balance fresh after purchases in other pages or tabs.
  if (auth.isLoggedIn) void auth.fetchMe().catch(() => undefined)
}

function onMenuHide() {
  menuOpen.value = false
}

async function logout() {
  await auth.logout()
  await router.push({ name: 'home' })
}
</script>

<template>
  <div v-if="auth.isLoggedIn && auth.user" class="user-menu">
    <Button
      text
      class="user-menu__trigger"
      :aria-label="`用户菜单：${auth.user.nickname}`"
      aria-haspopup="menu"
      :aria-expanded="menuOpen"
      @click="toggleMenu"
    >
      <Avatar
        :image="auth.user.avatarUrl || undefined"
        :label="auth.user.nickname.slice(0, 1)"
        shape="square"
        class="user-menu__avatar"
      />
      <span class="user-menu__name">{{ auth.user.nickname }}</span>
      <i class="pi pi-chevron-down user-menu__chevron" aria-hidden="true" />
    </Button>
    <Menu
      ref="menu"
      :model="menuItems"
      popup
      class="user-menu__popup"
      @show="onMenuShow"
      @hide="onMenuHide"
    >
      <template #start>
        <div class="user-menu__summary">
          <div class="user-menu__identity">
            <strong>{{ auth.user.nickname }}</strong>
            <span>@{{ auth.user.username }}</span>
          </div>
          <div class="user-menu__coins">
            <span>金币余额</span>
            <strong class="num">{{ formatCoins(auth.user.coins) }}</strong>
          </div>
        </div>
      </template>
    </Menu>
  </div>
  <div v-else class="user-menu__guest">
    <Button as="router-link" to="/login" label="登录" text size="small" />
    <Button as="router-link" to="/register" label="注册" size="small" />
  </div>
</template>

<style scoped>
.user-menu {
  display: inline-flex;
  min-width: 0;
}

.user-menu__trigger {
  display: inline-flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  min-width: 0;
  padding: 4px var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  border-radius: var(--kapoj-radius-sm);
}

.user-menu__trigger:hover {
  color: var(--kapoj-text);
  background: var(--kapoj-surface-hover);
}

.user-menu__avatar {
  flex: none;
  width: 28px;
  height: 28px;
  color: var(--kapoj-primary);
  font-size: 12px;
  font-weight: 700;
  background: var(--kapoj-primary-soft);
  border: 1px solid var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-sm);
}

.user-menu__name {
  max-width: 120px;
  overflow: hidden;
  font-size: 13px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-menu__chevron {
  flex: none;
  color: var(--kapoj-text-muted);
  font-size: 10px;
}

.user-menu__summary {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  min-width: 200px;
  padding: var(--kapoj-space-3);
  border-bottom: 1px solid var(--kapoj-border);
}

.user-menu__identity {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.user-menu__identity strong {
  color: var(--kapoj-text);
  font-size: 13px;
}

.user-menu__identity span {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.user-menu__coins {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.user-menu__coins strong {
  color: var(--kapoj-warning);
  font-size: 13px;
}

.user-menu__guest {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-1);
}

@media (max-width: 640px) {
  .user-menu__name {
    display: none;
  }
}
</style>
