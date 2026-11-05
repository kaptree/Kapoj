<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { claimTask, listTasks } from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { TaskItem, TaskStatus } from '@/api/types'
import MarkdownView from '@/components/MarkdownView.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/theme/theme'
import { formatDateTime } from '@/utils/format'
import { formatCoins, taskStatusLabel } from '@/utils/gamification'

const route = useRoute()
const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()
const { isDark } = useTheme()

const tasks = ref<TaskItem[]>([])
const loading = ref(false)
const error = ref('')
const claimingId = ref<number | null>(null)
let latest = 0

const isLoggedIn = computed(() => auth.isLoggedIn)
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listTasks()
    if (seq !== latest) return
    tasks.value = data
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载任务失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function progressPercent(task: TaskItem): number {
  if (task.problemCount <= 0) return task.status === 'completed' ? 100 : 0
  return Math.min(100, Math.round((task.progress / task.problemCount) * 100))
}

function taskSeverity(status: TaskStatus): 'success' | 'warn' | 'danger' | 'info' | 'secondary' {
  if (status === 'active' || status === 'completed') return 'success'
  if (status === 'claimed') return 'warn'
  if (status === 'expired') return 'danger'
  if (status === 'inactive') return 'secondary'
  return 'info'
}

function claim(task: TaskItem) {
  confirm.require({
    header: '领取任务',
    message: `确定领取任务「${task.title}」吗？领取后需在截止时间前完成全部题目。`,
    icon: 'pi pi-info-circle',
    acceptLabel: '领取',
    rejectLabel: '取消',
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void doClaim(task)
    },
  })
}

async function doClaim(task: TaskItem) {
  claimingId.value = task.id
  try {
    await claimTask(task.id)
    toast.add({
      severity: 'success',
      summary: '领取成功，完成全部题目即可获得奖励',
      life: 4000,
    })
    if (auth.isLoggedIn) await auth.fetchMe().catch(() => undefined)
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '领取失败',
      detail: e instanceof ApiError ? e.message : '领取失败，请稍后再试',
      life: 4000,
    })
  } finally {
    claimingId.value = null
  }
}

onMounted(load)
// Re-fetch when the user logs in or out so claim and progress state stay correct.
watch(
  () => auth.isLoggedIn,
  () => void load(),
)
</script>

<template>
  <div class="tasks-page">
    <AppPageHeader
      eyebrow="TASKS // BOUNTY"
      title="任务大厅"
      description="领取任务，在截止前 AC 全部题目即可获得金币奖励。"
    >
      <template #actions>
        <Tag v-if="isLoggedIn" severity="info" value="进度只统计领取后的 AC" />
      </template>
    </AppPageHeader>

    <div v-if="!isLoggedIn" class="login-cta">
      <span>登录后即可领取任务并记录完成进度。</span>
      <RouterLink :to="loginTarget">
        <Button label="登录 / 注册" icon="pi pi-sign-in" size="small" outlined />
      </RouterLink>
    </div>

    <AppErrorState v-if="error && !tasks.length" :description="error" @retry="load" />

    <AppLoadingState v-else-if="loading && !tasks.length" label="正在加载任务" />

    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

      <AppEmptyState
        v-if="!error && tasks.length === 0"
        icon="pi pi-briefcase"
        title="暂无任务"
        description="新的任务发布后会显示在这里。"
      />

      <div v-else class="task-grid">
        <AppSurface
          v-for="task in tasks"
          :key="task.id"
          as="article"
          padding="md"
          class="task-card"
          :class="{ 'is-completed': task.status === 'completed' }"
        >
          <div class="card-head">
            <div class="title-wrap">
              <h2 class="task-title">{{ task.title }}</h2>
              <div class="task-meta">
                <span>{{ task.problemCount }} 题</span>
                <span class="reward">
                  奖励 <strong>{{ formatCoins(task.rewardCoins) }}</strong> 金币
                </span>
              </div>
            </div>
            <Tag
              :value="taskStatusLabel(task.status)"
              :severity="taskSeverity(task.status)"
              class="task-status"
            />
          </div>

          <div v-if="task.descriptionMd.trim()" class="task-desc">
            <MarkdownView :source="task.descriptionMd" :theme="isDark ? 'dark' : 'light'" />
          </div>
          <div v-else class="muted">暂无描述</div>

          <div class="task-times">
            <div><span class="label">开始</span>{{ formatDateTime(task.startTime) }}</div>
            <div><span class="label">截止</span>{{ formatDateTime(task.endTime) }}</div>
            <div v-if="task.completedAt">
              <span class="label">完成</span>{{ formatDateTime(task.completedAt) }}
            </div>
          </div>

          <div v-if="task.claimedAt" class="progress-block">
            <div class="progress-head">
              <span>完成进度</span>
              <span class="num">{{ task.progress }} / {{ task.problemCount }}</span>
            </div>
            <ProgressBar
              :value="progressPercent(task)"
              :show-value="false"
              :class="{ 'is-done': task.status === 'completed' }"
            />
          </div>

          <div class="card-foot">
            <template v-if="task.status === 'active'">
              <RouterLink v-if="!isLoggedIn" :to="loginTarget">
                <Button label="登录后领取" icon="pi pi-sign-in" size="small" outlined />
              </RouterLink>
              <Button
                v-else
                label="领取任务"
                icon="pi pi-check"
                size="small"
                :loading="claimingId === task.id"
                :disabled="claimingId !== null && claimingId !== task.id"
                @click="claim(task)"
              />
            </template>
            <span v-else-if="task.status === 'claimed'" class="hint">
              完成全部题目后自动发放奖励
            </span>
            <span v-else-if="task.status === 'completed'" class="hint success"> 奖励已发放 </span>
            <span v-else-if="task.status === 'expired'" class="hint"> 任务已截止，未能完成 </span>
            <span v-else-if="task.status === 'upcoming'" class="hint"> 任务尚未开始 </span>
            <span v-else-if="task.status === 'ended'" class="hint"> 任务已结束 </span>
            <span v-else class="hint">任务已下线</span>
          </div>
        </AppSurface>
      </div>
    </template>
  </div>
</template>

<style scoped>
.tasks-page {
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

.task-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: var(--kapoj-space-4);
}

.task-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.task-card.is-completed {
  border-color: color-mix(in srgb, var(--kapoj-success) 45%, var(--kapoj-border));
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--kapoj-success) 18%, transparent);
}

.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.title-wrap {
  flex: 1;
  min-width: 0;
}

.task-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  line-height: 1.35;
  word-break: break-word;
}

.task-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-3);
  margin-top: var(--kapoj-space-1);
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.reward strong {
  color: var(--kapoj-warning, var(--kapoj-primary));
}

.task-status {
  flex: 0 0 auto;
}

.task-desc {
  max-height: 156px;
  overflow: auto;
  border-radius: var(--kapoj-radius-sm);
}

.task-desc :deep(.md-editor-preview-wrapper) {
  padding: 0;
}

.task-desc :deep(.md-editor-preview) {
  font-size: 13px;
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 13px;
}

.task-times {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  padding-top: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  border-top: 1px solid var(--kapoj-border);
}

.label {
  display: inline-block;
  width: 36px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-sans);
}

.progress-head {
  display: flex;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  margin-bottom: var(--kapoj-space-1);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.progress-block :deep(.p-progressbar) {
  height: 6px;
  background: var(--kapoj-bg);
  border-radius: var(--kapoj-radius-xs);
}

.progress-block :deep(.p-progressbar-value) {
  background: var(--kapoj-primary);
}

.progress-block :deep(.is-done .p-progressbar-value),
.progress-block :deep(.p-progressbar.is-done .p-progressbar-value) {
  background: var(--kapoj-success);
}

.card-foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: var(--kapoj-space-2);
  min-height: 40px;
  padding-top: var(--kapoj-space-3);
  margin-top: auto;
}

.hint {
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.hint.success {
  color: var(--kapoj-success);
}

@media (max-width: 640px) {
  .task-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .login-cta :deep(.p-button) {
    width: 100%;
  }
}
</style>
