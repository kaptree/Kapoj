<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'

import { claimTask, listTasks } from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { TaskItem } from '@/api/types'
import MarkdownView from '@/components/MarkdownView.vue'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'
import { formatCoins, taskStatusLabel, taskStatusTagType } from '@/utils/gamification'

const route = useRoute()
const auth = useAuthStore()

const tasks = ref<TaskItem[]>([])
const loading = ref(false)
const error = ref('')
const claimingId = ref<number | null>(null)
let latest = 0

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

async function claim(task: TaskItem) {
  try {
    await ElMessageBox.confirm(
      `确定领取任务「${task.title}」吗？领取后需在截止时间前完成全部题目。`,
      '领取任务',
      {
        type: 'info',
        confirmButtonText: '领取',
        cancelButtonText: '取消',
      },
    )
  } catch {
    return
  }

  claimingId.value = task.id
  try {
    await claimTask(task.id)
    ElMessage.success('领取成功，完成全部题目即可获得奖励')
    if (auth.isLoggedIn) await auth.fetchMe().catch(() => undefined)
    await load()
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '领取失败，请稍后再试')
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
  <el-card shadow="never">
    <template #header>
      <div class="head">
        <div>
          <span class="page-title">任务大厅</span>
          <span class="sub">领取任务，在截止前 AC 全部题目即可获得金币奖励</span>
        </div>
        <el-tag v-if="auth.isLoggedIn" type="info" effect="plain"> 进度只统计领取后的 AC </el-tag>
      </div>
    </template>

    <div v-if="!auth.isLoggedIn" class="cta">
      <span>登录后即可领取任务并记录完成进度。</span>
      <RouterLink :to="loginTarget">
        <el-button type="primary" plain size="small">登录 / 注册</el-button>
      </RouterLink>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />

    <div v-loading="loading" class="grid-wrap">
      <el-empty v-if="!loading && !error && tasks.length === 0" description="暂无任务" />
      <div v-else class="grid">
        <el-card
          v-for="task in tasks"
          :key="task.id"
          shadow="hover"
          class="task-card"
          :class="{ done: task.status === 'completed' }"
        >
          <div class="card-head">
            <div class="title-wrap">
              <div class="task-title">{{ task.title }}</div>
              <div class="meta">
                <span>{{ task.problemCount }} 题</span>
                <span class="reward"
                  >奖励 <strong>{{ formatCoins(task.rewardCoins) }}</strong> 金币</span
                >
              </div>
            </div>
            <el-tag :type="taskStatusTagType(task.status)" size="small">
              {{ taskStatusLabel(task.status) }}
            </el-tag>
          </div>

          <div v-if="task.descriptionMd.trim()" class="desc">
            <MarkdownView :source="task.descriptionMd" />
          </div>
          <div v-else class="muted">暂无描述</div>

          <div class="times">
            <div><span class="label">开始</span>{{ formatDateTime(task.startTime) }}</div>
            <div><span class="label">截止</span>{{ formatDateTime(task.endTime) }}</div>
            <div v-if="task.completedAt">
              <span class="label">完成</span>{{ formatDateTime(task.completedAt) }}
            </div>
          </div>

          <div v-if="task.claimedAt" class="progress-block">
            <div class="progress-head">
              <span>完成进度</span>
              <span>{{ task.progress }} / {{ task.problemCount }}</span>
            </div>
            <el-progress
              :percentage="progressPercent(task)"
              :status="task.status === 'completed' ? 'success' : undefined"
              :stroke-width="8"
              :show-text="false"
            />
          </div>

          <div class="card-foot">
            <template v-if="task.status === 'active'">
              <RouterLink v-if="!auth.isLoggedIn" :to="loginTarget">
                <el-button type="primary" plain size="small">登录后领取</el-button>
              </RouterLink>
              <el-button
                v-else
                type="primary"
                :loading="claimingId === task.id"
                :disabled="claimingId !== null && claimingId !== task.id"
                @click="claim(task)"
              >
                领取任务
              </el-button>
            </template>
            <span v-else-if="task.status === 'claimed'" class="hint"
              >完成全部题目后自动发放奖励</span
            >
            <span v-else-if="task.status === 'completed'" class="hint success">奖励已发放</span>
            <span v-else-if="task.status === 'expired'" class="hint">任务已截止，未能完成</span>
            <span v-else-if="task.status === 'upcoming'" class="hint">任务尚未开始</span>
            <span v-else-if="task.status === 'ended'" class="hint">任务已结束</span>
            <span v-else class="hint">任务已下线</span>
          </div>
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
.banner {
  margin-bottom: 12px;
}
.grid-wrap {
  min-height: 180px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}
.task-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.task-card.done {
  border-color: var(--el-color-success);
}
.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.title-wrap {
  flex: 1;
  min-width: 0;
}
.task-title {
  font-size: 16px;
  font-weight: 600;
  word-break: break-word;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 6px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.reward strong {
  color: var(--el-color-warning);
}
.desc {
  max-height: 156px;
  margin-top: 12px;
  overflow: auto;
  border-radius: 4px;
}
.desc :deep(.md-editor-preview-wrapper) {
  padding: 0;
}
.desc :deep(.md-editor-preview) {
  font-size: 13px;
}
.muted {
  margin-top: 12px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.times {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-top: 12px;
  margin-top: 12px;
  color: var(--el-text-color-regular);
  font-size: 12px;
  border-top: 1px solid var(--el-border-color-lighter);
}
.label {
  display: inline-block;
  width: 36px;
  color: var(--el-text-color-secondary);
}
.progress-block {
  margin-top: 12px;
}
.progress-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 4px;
  color: var(--el-text-color-regular);
  font-size: 12px;
}
.card-foot {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  min-height: 40px;
  margin-top: auto;
  padding-top: 12px;
}
.hint {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.hint.success {
  color: var(--el-color-success);
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
