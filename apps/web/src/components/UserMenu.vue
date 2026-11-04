<script setup lang="ts">
import { useRouter } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()

async function onCommand(command: string) {
  if (command === 'profile' && auth.user) {
    await router.push({ name: 'user-profile', params: { id: auth.user.id } })
  } else if (command === 'edit') {
    await router.push({ name: 'profile-edit' })
  } else if (command === 'logout') {
    await auth.logout()
    await router.push({ name: 'home' })
  }
}
</script>

<template>
  <el-dropdown v-if="auth.isLoggedIn && auth.user" trigger="click" @command="onCommand">
    <el-button text>
      <el-avatar :size="28" :src="auth.user.avatarUrl || undefined" class="avatar">
        {{ auth.user.nickname.slice(0, 1) }}
      </el-avatar>
      {{ auth.user.nickname }}
    </el-button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item command="profile">我的主页</el-dropdown-item>
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
</style>
