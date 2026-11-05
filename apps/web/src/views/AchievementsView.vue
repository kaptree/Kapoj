<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { listAchievements } from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { AchievementConditionProgress, AchievementItem } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'
import { achievementConditionLabel, badgeInitial, formatCoins } from '@/utils/gamification'

type Filter = 'all' | 'unlocked' | 'locked'

const route = useRoute()
const auth = useAuthStore()

const achievements = ref<AchievementItem[]>([])
const loading = ref(false)
const error = ref('')
const filter = ref<Filter>('all')
let latest = 0

const unlockedCount = computed(() => achievements.value.filter((item) => item.unlocked).length)
const filtered = computed(() => {
  if (filter.value === 'unlocked') return achievements.value.filter((item) => item.unlocked)
  if (filter.value === 'locked') return achievements.value.filter((item) => !item.unlocked)
  return achievements.value
})
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listAchievements()
    if (seq !== latest) return
    achievements.value = data
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载成就失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function conditionPercent(progress: AchievementConditionProgress): number {
  if (progress.target <= 0) return progress.done ? 100 : 0
  return Math.min(100, Math.round((progress.current / progress.target) * 100))
}

onMounted(load)
// Re-fetch when the user logs in or out so unlock state and progress stay correct.
watch(
  () => auth.isLoggedIn,
  () => void load(),
)
</script>

<template>
  <el-card shadow="never">
    <template #header>
      <div class="head">
        <div>
          <span class="page-title">成就墙</span>
          <span class="sub">完成条件即可解锁成就并获得奖励</span>
        </div>
        <el-tag type="success" effect="plain">
          已解锁 {{ unlockedCount }} / {{ achievements.length }}
        </el-tag>
      </div>
    </template>

    <div v-if="!auth.isLoggedIn" class="cta">
      <span>登录后可查看自己的成就解锁进度并领取奖励。</span>
      <RouterLink :to="loginTarget">
        <el-button type="primary" plain size="small">登录 / 注册</el-button>
      </RouterLink>
    </div>

    <div class="toolbar">
      <el-radio-group v-model="filter">
        <el-radio-button value="all">全部（{{ achievements.length }}）</el-radio-button>
        <el-radio-button value="unlocked">已解锁（{{ unlockedCount }}）</el-radio-button>
        <el-radio-button value="locked"
          >未解锁（{{ achievements.length - unlockedCount }}）</el-radio-button
        >
      </el-radio-group>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />

    <div v-loading="loading" class="grid-wrap">
      <el-empty
        v-if="!loading && !error && filtered.length === 0"
        description="暂无符合条件的成就"
      />
      <div v-else class="grid">
        <el-card
          v-for="item in filtered"
          :key="item.id"
          shadow="hover"
          class="achievement-card"
          :class="{ unlocked: item.unlocked }"
        >
          <div class="card-head">
            <el-avatar :size="52" shape="square" :src="item.iconUrl || undefined" class="icon">
              {{ badgeInitial(item.name) }}
            </el-avatar>
            <div class="name-wrap">
              <div class="name">{{ item.name }}</div>
              <div class="desc">{{ item.description || '暂无描述' }}</div>
            </div>
            <el-tag
              :type="item.unlocked ? 'success' : 'info'"
              size="small"
              effect="plain"
              class="status"
            >
              {{ item.unlocked ? '已解锁' : '未解锁' }}
            </el-tag>
          </div>

          <div class="reward">
            <span class="label">奖励</span>
            <el-tag v-if="item.rewardCoins > 0" type="warning" size="small">
              金币 +{{ formatCoins(item.rewardCoins) }}
            </el-tag>
            <el-tag v-if="item.rewardBadgeId !== null" type="primary" size="small">
              徽章 #{{ item.rewardBadgeId }}
            </el-tag>
            <span v-if="item.rewardCoins <= 0 && item.rewardBadgeId === null" class="muted"
              >无额外奖励</span
            >
          </div>

          <div v-if="item.unlocked" class="unlocked-at">
            <span class="label">解锁时间</span>
            <span>{{ formatDateTime(item.unlockedAt) }}</span>
          </div>
          <div v-else-if="item.progress.length" class="progress-list">
            <div v-for="(progress, index) in item.progress" :key="index" class="progress-item">
              <div class="progress-label">
                <span>{{ achievementConditionLabel(progress.type) }}</span>
                <span>{{ progress.current }} / {{ progress.target }}</span>
              </div>
              <el-progress
                :percentage="conditionPercent(progress)"
                :status="progress.done ? 'success' : undefined"
                :stroke-width="8"
                :show-text="false"
              />
            </div>
          </div>
          <div v-else class="muted">暂无进度信息</div>
        </el-card>
      </div>
    </div>
  </el-card>
</template>

<style scoped>
.head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.page-title {
  font-size: 18px;
  font-weight: 600;
}
.sub {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.cta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 14px;
  margin-bottom: 16px;
  color: var(--el-text-color-regular);
  background: var(--el-fill-color-light);
  border-radius: 6px;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}
.banner {
  margin-bottom: 12px;
}
.grid-wrap {
  min-height: 180px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}
.achievement-card {
  height: 100%;
}
.achievement-card.unlocked {
  border-color: var(--el-color-success);
}
.card-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}
.icon {
  flex: none;
  font-size: 20px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.name-wrap {
  flex: 1;
  min-width: 0;
}
.name {
  font-size: 16px;
  font-weight: 600;
  word-break: break-word;
}
.desc {
  margin-top: 4px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.5;
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
}
.status {
  flex: none;
}
.reward {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding-top: 12px;
  margin-top: 12px;
  border-top: 1px solid var(--el-border-color-lighter);
}
.label {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.muted {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.unlocked-at {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  color: var(--el-text-color-regular);
  font-size: 13px;
}
.progress-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 14px;
}
.progress-label {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 4px;
  color: var(--el-text-color-regular);
  font-size: 12px;
}
@media (max-width: 640px) {
  .grid {
    grid-template-columns: 1fr;
  }
  .sub {
    display: block;
    margin: 4px 0 0;
  }
}
</style>
