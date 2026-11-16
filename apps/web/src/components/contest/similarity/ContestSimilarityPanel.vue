<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import {
  getSimilarityOverview,
  startSimilarityRuns,
  updateSimilaritySettings,
} from '@/api/similarity'
import type {
  ContestSimilarityOverview,
  ContestSimilarityProblem,
  ContestSimilarityRun,
  ContestSimilaritySettingsInput,
} from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import ContestSimilaritySettingsDialog from './ContestSimilaritySettingsDialog.vue'

const POLL_INTERVAL = 2000

const props = defineProps<{ contestId: number }>()

const toast = useToast()

const overview = ref<ContestSimilarityOverview | null>(null)
const loading = ref(false)
const error = ref('')
const runningAll = ref(false)
const runningProblemId = ref<number | null>(null)
const settingsVisible = ref(false)
const settingsProblem = ref<ContestSimilarityProblem | null>(null)
const savingSettings = ref(false)

let pollTimer: number | undefined
let loadingOverview = false

const problems = computed(() => overview.value?.problems ?? [])
const hasActiveRuns = computed(() =>
  problems.value.some((problem) => isRunActive(problem.latestRun)),
)

const runStatusLabels: Record<string, string> = {
  queued: '排队中',
  running: '运行中',
  completed: '已完成',
  failed: '失败',
}

const runStatusSeverities: Record<string, 'success' | 'info' | 'warn' | 'danger' | 'secondary'> = {
  queued: 'warn',
  running: 'info',
  completed: 'success',
  failed: 'danger',
}

function isRunActive(run: ContestSimilarityRun | null): boolean {
  return run?.status === 'queued' || run?.status === 'running'
}

function runStatusLabel(run: ContestSimilarityRun | null): string {
  if (!run) return '未运行'
  return runStatusLabels[run.status] ?? run.status
}

function runStatusSeverity(
  run: ContestSimilarityRun | null,
): 'success' | 'info' | 'warn' | 'danger' | 'secondary' {
  if (!run) return 'secondary'
  return runStatusSeverities[run.status] ?? 'secondary'
}

function failureMessage(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

function notifyError(e: unknown, fallback: string) {
  toast.add({
    severity: 'error',
    summary: '操作失败',
    detail: failureMessage(e, fallback),
    life: 4000,
  })
}

function stopPolling() {
  if (pollTimer !== undefined) {
    window.clearInterval(pollTimer)
    pollTimer = undefined
  }
}

function syncPolling() {
  if (hasActiveRuns.value) {
    if (pollTimer === undefined) {
      pollTimer = window.setInterval(() => {
        void loadOverview(true)
      }, POLL_INTERVAL)
    }
  } else {
    stopPolling()
  }
}

async function loadOverview(background = false) {
  if (loadingOverview) return
  loadingOverview = true
  if (!background) {
    loading.value = true
    error.value = ''
  }
  try {
    overview.value = await getSimilarityOverview(props.contestId)
    error.value = ''
  } catch (e) {
    const message = failureMessage(e, '加载查重概览失败，请稍后再试')
    error.value = background && overview.value ? '刷新查重状态失败，稍后将自动重试' : message
  } finally {
    loadingOverview = false
    if (!background) loading.value = false
    syncPolling()
  }
}

function openSettings(problem: ContestSimilarityProblem) {
  settingsProblem.value = problem
  settingsVisible.value = true
}

function applySettings(problemId: number, saved: ContestSimilaritySettingsInput) {
  if (!overview.value) return
  overview.value = {
    ...overview.value,
    problems: overview.value.problems.map((problem) =>
      problem.problemId === problemId
        ? {
            ...problem,
            thresholdPercent: saved.thresholdPercent,
            ignoreCommonCode: saved.ignoreCommonCode,
            baseLanguage: saved.baseLanguage,
          }
        : problem,
    ),
  }
}

async function saveSettings(payload: ContestSimilaritySettingsInput) {
  const problem = settingsProblem.value
  if (!problem || savingSettings.value) return
  savingSettings.value = true
  try {
    const saved = await updateSimilaritySettings(props.contestId, problem.problemId, payload)
    applySettings(problem.problemId, {
      thresholdPercent: saved.thresholdPercent,
      ignoreCommonCode: saved.ignoreCommonCode,
      baseLanguage: saved.baseLanguage,
      baseCode: saved.baseCode,
    })
    settingsVisible.value = false
    toast.add({ severity: 'success', summary: '查重设置已保存', life: 3000 })
  } catch (e) {
    notifyError(e, '保存查重设置失败')
  } finally {
    savingSettings.value = false
  }
}

async function runProblem(problem: ContestSimilarityProblem) {
  if (runningProblemId.value !== null) return
  runningProblemId.value = problem.problemId
  try {
    await startSimilarityRuns(props.contestId, problem.problemId)
    toast.add({ severity: 'success', summary: '已开始本题查重', life: 3000 })
    await loadOverview(true)
  } catch (e) {
    notifyError(e, '启动本题查重失败')
  } finally {
    runningProblemId.value = null
  }
}

async function runAll() {
  if (runningAll.value) return
  runningAll.value = true
  try {
    await startSimilarityRuns(props.contestId)
    toast.add({ severity: 'success', summary: '已开始全部查重', life: 3000 })
    await loadOverview(true)
  } catch (e) {
    notifyError(e, '启动全部查重失败')
  } finally {
    runningAll.value = false
  }
}

onMounted(() => {
  void loadOverview()
})

onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="similarity-panel">
    <AppLoadingState v-if="loading && !overview" label="加载查重概览" :rows="3" />
    <AppErrorState v-else-if="error && !overview" :description="error" @retry="loadOverview()" />

    <template v-else>
      <AppSurface class="similarity-overview">
        <div class="similarity-toolbar">
          <div class="similarity-heading">
            <h2 class="similarity-title">代码查重</h2>
            <span class="similarity-count">共 {{ problems.length }} 道题目</span>
          </div>
          <Button
            label="计算全部"
            data-test="similarity-run-all"
            icon="pi pi-play"
            size="small"
            :loading="runningAll"
            :disabled="problems.length === 0"
            @click="runAll"
          />
        </div>

        <Message v-if="error" severity="error" :closable="false" class="similarity-message">
          {{ error }}
        </Message>

        <div v-if="problems.length" class="similarity-table-wrap">
          <table class="similarity-table">
            <caption class="sr-only">
              赛题查重设置与最近运行状态
            </caption>
            <thead>
              <tr>
                <th scope="col">题号</th>
                <th scope="col">题目</th>
                <th scope="col">代码提交数</th>
                <th scope="col">阈值</th>
                <th scope="col">最近状态</th>
                <th scope="col">结果数</th>
                <th scope="col">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="problem in problems"
                :key="problem.problemId"
                data-test="similarity-problem-row"
              >
                <td>
                  <span class="similarity-id">{{ problem.displayId }}</span>
                </td>
                <td>
                  <span class="similarity-problem-title">{{ problem.title }}</span>
                </td>
                <td class="num">{{ problem.submissionCount }}</td>
                <td>
                  <span class="num">{{ problem.thresholdPercent }}%</span>
                  <Tag
                    v-if="problem.ignoreCommonCode"
                    value="忽略公共代码"
                    severity="secondary"
                    class="similarity-flag"
                  />
                </td>
                <td>
                  <Tag
                    :value="runStatusLabel(problem.latestRun)"
                    :severity="runStatusSeverity(problem.latestRun)"
                    :title="problem.latestRun?.errorMessage || undefined"
                  />
                </td>
                <td class="num">{{ problem.matchCount }}</td>
                <td>
                  <div class="row-actions">
                    <Button
                      label="设置"
                      data-test="similarity-settings-open"
                      text
                      size="small"
                      @click="openSettings(problem)"
                    />
                    <Button
                      label="计算本题"
                      data-test="similarity-run-problem"
                      text
                      size="small"
                      :loading="runningProblemId === problem.problemId"
                      :disabled="isRunActive(problem.latestRun)"
                      @click="runProblem(problem)"
                    />
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <AppEmptyState
          v-else
          icon="pi pi-code"
          title="暂无可查重题目"
          description="比赛包含代码题目后才能进行查重。"
        />
      </AppSurface>
    </template>

    <ContestSimilaritySettingsDialog
      v-model:visible="settingsVisible"
      :problem="settingsProblem"
      :saving="savingSettings"
      @save="saveSettings"
    />
  </div>
</template>

<style scoped>
.similarity-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.similarity-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  margin-bottom: var(--kapoj-space-3);
}

.similarity-heading {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.similarity-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.similarity-count {
  color: var(--kapoj-text-muted);
  font-size: 0.8125rem;
}

.similarity-message {
  margin-bottom: var(--kapoj-space-3);
}

.similarity-table-wrap {
  overflow-x: auto;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.similarity-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
}

.similarity-table th {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-align: left;
  text-transform: uppercase;
  white-space: nowrap;
  background: var(--kapoj-bg-elevated);
  border-bottom: 1px solid var(--kapoj-border);
}

.similarity-table td {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  vertical-align: middle;
  border-bottom: 1px solid var(--kapoj-border);
}

.similarity-table tbody tr:last-child td {
  border-bottom: none;
}

.similarity-table tbody tr:hover {
  background: var(--kapoj-surface-hover);
}

.similarity-id {
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-weight: 600;
}

.similarity-problem-title {
  color: var(--kapoj-text);
}

.similarity-flag {
  margin-left: var(--kapoj-space-1);
}

.num {
  font-family: var(--kapoj-font-mono);
  font-variant-numeric: tabular-nums;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-1);
}
</style>
