<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'

import { listAchievements } from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { AchievementConditionProgress, AchievementItem } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'
import {
  achievementConditionLabel,
  badgeImageSrc,
  badgeInitial,
  formatCoins,
} from '@/utils/gamification'

type Filter = 'all' | 'unlocked' | 'locked'

const route = useRoute()
const auth = useAuthStore()

const achievements = ref<AchievementItem[]>([])
const loading = ref(false)
const error = ref('')
const filter = ref<Filter>('all')
const failedIcons = ref<number[]>([])
let latest = 0

const isLoggedIn = computed(() => auth.isLoggedIn)
const unlockedCount = computed(() => achievements.value.filter((item) => item.unlocked).length)
const filtered = computed(() => {
  if (filter.value === 'unlocked') return achievements.value.filter((item) => item.unlocked)
  if (filter.value === 'locked') return achievements.value.filter((item) => !item.unlocked)
  return achievements.value
})
const filterOptions = computed(() => [
  { label: `全部（${achievements.value.length}）`, value: 'all' },
  { label: `已解锁（${unlockedCount.value}）`, value: 'unlocked' },
  { label: `未解锁（${achievements.value.length - unlockedCount.value}）`, value: 'locked' },
])
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listAchievements()
    if (seq !== latest) return
    achievements.value = data
    failedIcons.value = []
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

function iconSrc(item: AchievementItem): string | undefined {
  return failedIcons.value.includes(item.id) ? undefined : badgeImageSrc(item.iconUrl)
}

function onIconError(id: number) {
  if (!failedIcons.value.includes(id)) failedIcons.value.push(id)
}

onMounted(load)
// Re-fetch when the user logs in or out so unlock state and progress stay correct.
watch(
  () => auth.isLoggedIn,
  () => void load(),
)
</script>

<template>
  <div class="achievements-page">
    <AppPageHeader
      eyebrow="ACHIEVEMENTS // WALL"
      title="成就墙"
      description="完成条件即可解锁成就并获得奖励。"
    >
      <template #actions>
        <Tag severity="success" :value="`已解锁 ${unlockedCount} / ${achievements.length}`" />
      </template>
    </AppPageHeader>

    <div v-if="!isLoggedIn" class="login-cta">
      <span>登录后可查看自己的成就解锁进度；达成后奖励自动发放。</span>
      <RouterLink :to="loginTarget">
        <Button label="登录 / 注册" icon="pi pi-sign-in" size="small" outlined />
      </RouterLink>
    </div>

    <div class="toolbar">
      <SelectButton
        v-model="filter"
        :options="filterOptions"
        option-label="label"
        option-value="value"
        :allow-empty="false"
      />
    </div>

    <AppErrorState v-if="error && !achievements.length" :description="error" @retry="load" />

    <AppLoadingState v-else-if="loading && !achievements.length" label="正在加载成就" />

    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

      <AppEmptyState
        v-if="!error && filtered.length === 0"
        icon="pi pi-star"
        title="暂无符合条件的成就"
        description="切换筛选条件，或等待新的成就上线。"
      />

      <div v-else class="achievement-grid">
        <AppSurface
          v-for="item in filtered"
          :key="item.id"
          as="article"
          padding="md"
          class="achievement-card"
          :class="{ 'is-unlocked': item.unlocked }"
        >
          <div class="card-head">
            <Avatar
              :image="iconSrc(item)"
              :label="iconSrc(item) ? undefined : badgeInitial(item.name)"
              size="large"
              shape="square"
              class="achievement-icon"
              :aria-label="`成就图标：${item.name}`"
              @error="onIconError(item.id)"
            />
            <div class="name-wrap">
              <h2 class="achievement-name">{{ item.name }}</h2>
              <p class="achievement-desc">{{ item.description || '暂无描述' }}</p>
            </div>
            <Tag
              :value="item.unlocked ? '已解锁' : '未解锁'"
              :severity="item.unlocked ? 'success' : 'info'"
              class="achievement-status"
            />
          </div>

          <div class="achievement-reward">
            <span class="label">奖励</span>
            <Tag
              v-if="item.rewardCoins > 0"
              severity="warn"
              :value="`金币 +${formatCoins(item.rewardCoins)}`"
            />
            <Tag
              v-if="item.rewardBadgeId !== null"
              severity="info"
              :value="`徽章 #${item.rewardBadgeId}`"
            />
            <span v-if="item.rewardCoins <= 0 && item.rewardBadgeId === null" class="muted">
              无额外奖励
            </span>
          </div>

          <div v-if="item.unlocked" class="unlocked-at">
            <span class="label">解锁时间</span>
            <span>{{ item.unlockedAt ? formatDateTime(item.unlockedAt) : '' }}</span>
          </div>
          <div v-else-if="item.progress.length" class="progress-list">
            <div
              v-for="(progress, index) in item.progress"
              :key="index"
              class="progress-item"
              :class="{ 'is-done': progress.done }"
            >
              <div class="progress-label">
                <span>{{ achievementConditionLabel(progress.type) }}</span>
                <span class="num">{{ progress.current }} / {{ progress.target }}</span>
              </div>
              <ProgressBar :value="conditionPercent(progress)" :show-value="false" />
            </div>
          </div>
          <div v-else class="muted">暂无进度信息</div>
        </AppSurface>
      </div>
    </template>
  </div>
</template>

<style scoped>
.achievements-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.login-cta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 26%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-md);
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.achievement-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--kapoj-space-4);
}

.achievement-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.achievement-card.is-unlocked {
  border-color: color-mix(in srgb, var(--kapoj-success) 45%, var(--kapoj-border));
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--kapoj-success) 18%, transparent);
}

.card-head {
  display: flex;
  align-items: flex-start;
  gap: var(--kapoj-space-3);
}

.achievement-icon {
  flex: 0 0 auto;
  width: 52px;
  height: 52px;
  color: var(--kapoj-primary);
  font-size: 1.25rem;
  font-weight: 700;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 26%, transparent);
}

.name-wrap {
  flex: 1;
  min-width: 0;
}

.achievement-name {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  line-height: 1.35;
  word-break: break-word;
}

.achievement-desc {
  display: -webkit-box;
  margin: var(--kapoj-space-1) 0 0;
  overflow: hidden;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  line-height: 1.5;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
}

.achievement-status {
  flex: 0 0 auto;
}

.achievement-reward,
.unlocked-at {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding-top: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  border-top: 1px solid var(--kapoj-border);
}

.achievement-reward {
  margin-top: auto;
}

.label {
  color: var(--kapoj-text-muted);
  font-size: 11px;
  font-weight: 600;
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.progress-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.progress-label {
  display: flex;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  margin-bottom: var(--kapoj-space-1);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.progress-item :deep(.p-progressbar) {
  height: 6px;
  background: var(--kapoj-bg);
  border-radius: var(--kapoj-radius-xs);
}

.progress-item :deep(.p-progressbar-value) {
  background: var(--kapoj-primary);
}

.progress-item.is-done :deep(.p-progressbar-value) {
  background: var(--kapoj-success);
}

@media (max-width: 640px) {
  .achievement-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .login-cta :deep(.p-button) {
    width: 100%;
  }
}
</style>
