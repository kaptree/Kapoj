<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { ApiError } from '@/api/http'
import { getProfile } from '@/api/users'
import type { Profile } from '@/api/types'
import IdentityShowcase from '@/components/IdentityShowcase.vue'

const route = useRoute()
const profile = ref<Profile | null>(null)
const error = ref('')
const loading = ref(false)

watch(
  () => route.params.id,
  async (id) => {
    if (typeof id !== 'string') return
    loading.value = true
    error.value = ''
    profile.value = null
    try {
      profile.value = await getProfile(id)
    } catch (e) {
      error.value =
        e instanceof ApiError && e.code === 40400 ? '用户不存在' : '加载失败，请稍后再试'
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)
</script>

<template>
  <el-card v-loading="loading" shadow="never" class="card">
    <el-result v-if="error" icon="warning" :title="error" />
    <template v-else-if="profile">
      <div class="head">
        <el-avatar :size="64" :src="profile.avatarUrl || undefined">
          {{ profile.nickname.slice(0, 1) }}
        </el-avatar>
        <div class="identity">
          <div class="name-row">
            <h2>{{ profile.nickname }}</h2>
            <IdentityShowcase
              :badges="profile.equippedBadges"
              :title="profile.equippedTitle"
              size="small"
            />
          </div>
          <div class="username">@{{ profile.username }}</div>
        </div>
      </div>
      <p v-if="profile.bio">{{ profile.bio }}</p>
      <p v-else class="muted">这个人还没有写简介。</p>
      <el-descriptions :column="3" border>
        <el-descriptions-item label="Rating">{{ profile.rating }}</el-descriptions-item>
        <el-descriptions-item label="金币">{{ profile.coins }}</el-descriptions-item>
        <el-descriptions-item label="注册时间">
          {{ new Date(profile.createdAt).toLocaleDateString('zh-CN') }}
        </el-descriptions-item>
      </el-descriptions>
    </template>
  </el-card>
</template>

<style scoped>
.card {
  max-width: 720px;
  min-height: 200px;
}
.head {
  display: flex;
  align-items: center;
  gap: 16px;
}
.identity {
  min-width: 0;
}
.name-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.name-row h2 {
  margin: 0;
}
.username,
.muted {
  color: var(--el-text-color-secondary);
}
</style>
