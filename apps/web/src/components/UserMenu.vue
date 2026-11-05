<script setup lang="ts">
import { useRouter } from 'vue-router'

import { useAuthStore } from '@/stores/auth'
import { formatCoins } from '@/utils/gamification'

const auth = useAuthStore()
const router = useRouter()

async function onCommand(command: string) {
  if (command === 'profile' && auth.user) {
    await router.push({ name: 'user-profile', params: { id: auth.user.id } })
  } else if (command === 'appearance') {
    await router.push({ name: 'appearance' })
  } else if (command === 'edit') {
    await router.push({ name: 'profile-edit' })
  } else if (command === 'logout') {
    await auth.logout()
    await router.push({ name: 'home' })
  }
}

function onVisibleChange(visible: boolean) {
  // Keep the coin balance fresh after purchases in other pages or tabs.
  if (visible && auth.isLoggedIn) void auth.fetchMe().catch(() => undefined)
}
</script>

<template>
  <el-dropdown
    v-if="auth.isLoggedIn && auth.user"
    trigger="click"
    @command="onCommand"
    @visible-change="onVisibleChange"
  >
    <el-button text>
      <el-avatar :size="28" :src="auth.user.avatarUrl || undefined" class="avatar">
        {{ auth.user.nickname.slice(0, 1) }}
      </el-avatar>
      <span class="user-name">{{ auth.user.nickname }}</span>
    </el-button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item disabled class="coin-item">
          <span>金币余额</span>
          <strong class="coin-value">{{ formatCoins(auth.user.coins) }}</strong>
        </el-dropdown-item>
        <el-dropdown-item command="profile" divided>我的主页</el-dropdown-item>
        <el-dropdown-item command="appearance">装扮管理</el-dropdown-item>
        <el-dropdown-item command="edit">编辑资料</el-dropdown-item>
        <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
  <div v-else class="guest">
    <RouterLink to="/login"><el-button text>登录</el-button></RouterLink>
    <RouterLink to="/register"><el-button type="primary">注册</el-button></RouterLink>
  </div>
</template>

<style scoped>
.avatar {
  margin-right: 8px;
}
.guest {
  display: flex;
  gap: 8px;
}
.coin-item {
  display: flex;
  gap: 12px;
  justify-content: space-between;
  min-width: 150px;
}
.coin-item.is-disabled {
  color: var(--el-text-color-regular);
  cursor: default;
  opacity: 1;
}
.coin-value {
  color: var(--el-color-warning);
}
@media (max-width: 480px) {
  .user-name {
    display: none;
  }
}
</style>
