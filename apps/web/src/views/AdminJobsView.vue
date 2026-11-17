<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import ProgressBar from 'primevue/progressbar'
import Select from 'primevue/select'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  getAdminDifficultySettings,
  getAdminJobRun,
  listAdminJobRuns,
  listAdminJobs,
  runAdminJob,
  updateAdminDifficultySettings,
  updateAdminJob,
} from '@/api/admin'
import { ApiError } from '@/api/http'
import type {
  DifficultySettingsInput,
  DifficultySettingsView,
  JobRunView,
  JobView,
} from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatDateTime } from '@/utils/format'

const difficultyJobKey = 'problem.difficulty.recalculate'
const ratingJobKey = 'user.rating.recalculate'
const PAGE_SIZE = 20
const POLL_MS = 2500
const POLL_RETRY_MS = 5000

type TagSeverity = 'success' | 'secondary' | 'info' | 'warn' | 'danger' | 'contrast'
type ActiveTab = 'jobs' | 'difficulty' | 'history'

const ACTIVE_STATUSES = new Set(['queued', 'running'])

const STATUS_META: Record<string, { label: string; severity: TagSeverity }> = {
  queued: { label: '排队中', severity: 'info' },
  running: { label: '运行中', severity: 'info' },
  succeeded: { label: '成功', severity: 'success' },
  failed: { label: '失败', severity: 'danger' },
  canceled: { label: '已取消', severity: 'secondary' },
}

const STATUS_OPTIONS = [
  { label: '全部状态', value: '' },
  { label: '排队中', value: 'queued' },
  { label: '运行中', value: 'running' },
  { label: '成功', value: 'succeeded' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'canceled' },
]

const CRON_PRESETS = [
  { label: '每天 03:00', value: '0 3 * * *' },
  { label: '每周一 03:00', value: '0 3 * * 1' },
]

const toast = useToast()
const confirm = useConfirm()

const activeTab = ref<ActiveTab>('jobs')

// --- Job list ---------------------------------------------------------------

const jobs = ref<JobView[]>([])
const jobsLoading = ref(false)
const jobsError = ref('')
const pollError = ref('')
const latestRun = ref<JobRunView | null>(null)
const runningKey = ref('')

const ratingPlaceholderVisible = computed(() => !jobs.value.some((job) => job.key === ratingJobKey))

const jobOptions = computed(() => [
  { label: '全部任务', value: '' },
  ...jobs.value.map((job) => ({ label: job.name, value: job.key })),
])

const difficultyRunning = computed(() => runningKey.value === difficultyJobKey)

function isActiveStatus(status: string): boolean {
  return ACTIVE_STATUSES.has(status)
}

function isDifficultyJob(key: string): boolean {
  return key === difficultyJobKey
}

function statusLabel(status: string): string {
  if (!status) return '未运行'
  return STATUS_META[status]?.label ?? status
}

function statusSeverity(status: string): TagSeverity {
  if (!status) return 'secondary'
  return STATUS_META[status]?.severity ?? 'secondary'
}

function triggerLabel(trigger: string): string {
  if (trigger === 'manual') return '手动'
  if (trigger === 'schedule') return '定时'
  return trigger || '—'
}

function jobName(key: string): string {
  return jobs.value.find((job) => job.key === key)?.name ?? key
}

function progressPercent(run: JobRunView): number {
  if (run.progressTotal <= 0) return 0
  return Math.min(100, Math.round((run.progressCurrent / run.progressTotal) * 100))
}

function progressText(run: JobRunView): string {
  if (run.progressTotal > 0) return `${run.progressCurrent} / ${run.progressTotal}`
  return isActiveStatus(run.status) ? '等待进度' : '—'
}

function durationText(run: JobRunView): string {
  if (!run.startedAt) return '—'
  if (!run.finishedAt) return '运行中'
  const started = new Date(run.startedAt).getTime()
  const finished = new Date(run.finishedAt).getTime()
  if (Number.isNaN(started) || Number.isNaN(finished)) return '—'
  const seconds = Math.max(0, Math.round((finished - started) / 1000))
  if (seconds < 60) return `${seconds} 秒`
  return `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`
}

function statsText(stats: JobRunView['stats']): string {
  const entries = Object.entries(stats ?? {})
  if (entries.length === 0) return '—'
  return entries
    .map(([key, value]) => {
      const text =
        value !== null && typeof value === 'object' ? JSON.stringify(value) : String(value)
      return `${key}=${text}`
    })
    .join(' · ')
}

let jobsSeq = 0

async function loadJobs(silent = false): Promise<boolean> {
  const seq = ++jobsSeq
  if (!silent) {
    jobsLoading.value = true
    jobsError.value = ''
  }
  try {
    const data = await listAdminJobs()
    if (seq !== jobsSeq) return true
    jobs.value = data
    return true
  } catch (e) {
    if (seq !== jobsSeq) return true
    if (!silent) {
      jobsError.value = e instanceof ApiError ? e.message : '加载定时任务失败，请稍后再试'
    }
    return false
  } finally {
    if (seq === jobsSeq && !silent) jobsLoading.value = false
  }
}

// --- Polling ----------------------------------------------------------------

let pollTimer: ReturnType<typeof setTimeout> | undefined
let unmounted = false

const hasActiveRun = computed(() =>
  jobs.value.some((job) => job.activeRun !== null && isActiveStatus(job.activeRun.status)),
)

function clearPoll() {
  if (pollTimer !== undefined) {
    clearTimeout(pollTimer)
    pollTimer = undefined
  }
}

function schedulePoll(delay = POLL_MS) {
  clearPoll()
  if (unmounted || !hasActiveRun.value) return
  pollTimer = setTimeout(() => {
    pollTimer = undefined
    void pollOnce()
  }, delay)
}

async function syncLatestRun() {
  const current = latestRun.value
  if (!current || !isActiveStatus(current.status)) return
  const next = jobs.value
    .map((job) => job.activeRun)
    .find((run): run is JobRunView => run !== null && run.id === current.id)
  if (next) {
    latestRun.value = next
    return
  }
  // The run left queued/running between polls; fetch its terminal state.
  try {
    const finished = await getAdminJobRun(current.id)
    if (!unmounted && finished) latestRun.value = finished
  } catch {
    // Keep the last known state; the job card and history still show the result.
  }
}

function pollFailureMessage(reason: string): string {
  return hasActiveRun.value ? `${reason}，正在自动重试…` : `${reason}，请点击重试。`
}

async function pollOnce() {
  if (unmounted) return
  const ok = await loadJobs(true)
  if (unmounted) return
  if (!ok) {
    // A transient refresh failure must not strand the stale active run: keep
    // retrying with backoff, and expose a manual retry while it is visible.
    pollError.value = pollFailureMessage('刷新任务状态失败')
    if (hasActiveRun.value) schedulePoll(POLL_RETRY_MS)
    return
  }

  pollError.value = ''
  if (runsLoaded.value) {
    const runsOk = await loadRuns(true)
    if (unmounted) return
    if (!runsOk) pollError.value = pollFailureMessage('刷新运行历史失败')
  }
  await syncLatestRun()
  if (unmounted) return
  schedulePoll()
}

async function retryPoll() {
  clearPoll()
  pollError.value = ''
  const ok = await loadJobs()
  if (unmounted) return
  if (!ok) {
    pollError.value = '刷新任务状态失败，请稍后重试'
    if (hasActiveRun.value) schedulePoll(POLL_RETRY_MS)
    return
  }
  if (runsLoaded.value) await loadRuns()
  if (unmounted) return
  await syncLatestRun()
  if (unmounted) return
  schedulePoll()
}

// --- Manual runs ------------------------------------------------------------

async function executeRun(key: string, name: string) {
  runningKey.value = key
  try {
    const run = await runAdminJob(key)
    latestRun.value = run
    const job = jobs.value.find((item) => item.key === key)
    if (job && isActiveStatus(run.status)) job.activeRun = run
    toast.add({
      severity: 'success',
      summary: '任务已入队',
      detail: `${name} · 运行 #${run.id}`,
      life: 3000,
    })
    schedulePoll()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '执行失败',
      detail: e instanceof ApiError ? e.message : '立即执行失败，请稍后再试',
      life: 4000,
    })
  } finally {
    runningKey.value = ''
  }
}

function confirmRun(job: JobView) {
  confirm.require({
    header: '立即执行任务',
    message: `立即执行「${job.name}」？任务将进入队列异步执行。`,
    icon: 'pi pi-play',
    acceptLabel: '立即执行',
    rejectLabel: '取消',
    accept: () => {
      void executeRun(job.key, job.name)
    },
  })
}

// --- Schedule dialog --------------------------------------------------------

const editVisible = ref(false)
const editingJob = ref<JobView | null>(null)
const savingPlan = ref(false)
const planError = ref('')
const planForm = ref({
  enabled: true,
  cronExpr: '0 3 * * *',
  timezone: 'Asia/Shanghai',
})

function openEdit(job: JobView) {
  editingJob.value = job
  planError.value = ''
  planForm.value = {
    enabled: job.enabled,
    cronExpr: job.cronExpr,
    timezone: job.timezone || 'Asia/Shanghai',
  }
  editVisible.value = true
}

function applyCronPreset(value: string) {
  planForm.value.cronExpr = value
  planError.value = ''
}

function validatePlan(): string {
  const parts = planForm.value.cronExpr.trim().split(/\s+/)
  if (parts.length !== 5) return 'cron 表达式需为 5 段（分 时 日 月 周）'
  if (!planForm.value.timezone.trim()) return '时区不能为空'
  return ''
}

async function savePlan() {
  const job = editingJob.value
  if (!job) return
  const error = validatePlan()
  if (error) {
    planError.value = error
    return
  }
  savingPlan.value = true
  try {
    const updated = await updateAdminJob(job.key, {
      enabled: planForm.value.enabled,
      cronExpr: planForm.value.cronExpr.trim(),
      timezone: planForm.value.timezone.trim(),
    })
    const index = jobs.value.findIndex((item) => item.key === job.key)
    if (index >= 0) jobs.value[index] = updated
    toast.add({ severity: 'success', summary: '计划已更新', life: 3000 })
    editVisible.value = false
    schedulePoll()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: e instanceof ApiError ? e.message : '更新任务计划失败，请稍后再试',
      life: 4000,
    })
  } finally {
    savingPlan.value = false
  }
}

// --- Difficulty settings ----------------------------------------------------

const settings = ref<DifficultySettingsView | null>(null)
const settingsLoading = ref(false)
const settingsSaving = ref(false)
const settingsError = ref('')

const settingsForm = ref({
  enabled: true,
  defaultScore: 50 as number | null,
  minSample: 10 as number | null,
  priorStrength: 10 as number | null,
  maxStep: 5 as number | null,
  windowDays: 365 as number | null,
  includeContest: false,
  curveGamma: 1.5 as number | null,
  aiAssessEnabled: true,
  aiTimeoutSeconds: 15 as number | null,
})

const settingsErrors = ref({
  defaultScore: '',
  minSample: '',
  priorStrength: '',
  maxStep: '',
  windowDays: '',
  curveGamma: '',
  aiTimeoutSeconds: '',
})

function applySettings(view: DifficultySettingsView) {
  settings.value = view
  settingsForm.value.enabled = view.enabled
  settingsForm.value.defaultScore = view.defaultScore
  settingsForm.value.minSample = view.minSample
  settingsForm.value.priorStrength = view.priorStrength
  settingsForm.value.maxStep = view.maxStep
  settingsForm.value.windowDays = view.windowDays
  settingsForm.value.includeContest = view.includeContest
  settingsForm.value.curveGamma = view.curveGamma
  settingsForm.value.aiAssessEnabled = view.aiAssessEnabled
  settingsForm.value.aiTimeoutSeconds = view.aiTimeoutSeconds
}

async function loadSettings() {
  if (settingsLoading.value) return
  settingsLoading.value = true
  settingsError.value = ''
  try {
    applySettings(await getAdminDifficultySettings())
  } catch (e) {
    settingsError.value = e instanceof ApiError ? e.message : '加载难度设置失败，请稍后再试'
  } finally {
    settingsLoading.value = false
  }
}

function intError(value: number | null, min: number, max: number, label: string): string {
  if (value === null || !Number.isInteger(value) || value < min || value > max) {
    return `${label}必须在 ${min} 到 ${max} 之间`
  }
  return ''
}

function validateSettings(): boolean {
  settingsErrors.value.defaultScore = intError(
    settingsForm.value.defaultScore,
    1,
    100,
    '默认难度分',
  )
  settingsErrors.value.minSample = intError(settingsForm.value.minSample, 1, 1000, '最少样本数')
  settingsErrors.value.priorStrength = intError(
    settingsForm.value.priorStrength,
    1,
    100,
    '先验强度',
  )
  settingsErrors.value.maxStep = intError(settingsForm.value.maxStep, 1, 100, '每日步长')
  settingsErrors.value.windowDays = intError(settingsForm.value.windowDays, 0, 3650, '统计窗口天数')
  const gamma = settingsForm.value.curveGamma
  settingsErrors.value.curveGamma =
    gamma === null || Number.isNaN(gamma) || gamma < 1 || gamma > 3
      ? 'curve gamma 必须在 1.0 到 3.0 之间'
      : ''
  settingsErrors.value.aiTimeoutSeconds = intError(
    settingsForm.value.aiTimeoutSeconds,
    5,
    60,
    'AI 超时秒数',
  )
  return Object.values(settingsErrors.value).every((message) => message === '')
}

async function saveSettings() {
  if (!validateSettings()) return
  settingsSaving.value = true
  try {
    const input: DifficultySettingsInput = {
      enabled: settingsForm.value.enabled,
      defaultScore: settingsForm.value.defaultScore as number,
      minSample: settingsForm.value.minSample as number,
      priorStrength: settingsForm.value.priorStrength as number,
      maxStep: settingsForm.value.maxStep as number,
      windowDays: settingsForm.value.windowDays as number,
      includeContest: settingsForm.value.includeContest,
      curveGamma: settingsForm.value.curveGamma as number,
      aiAssessEnabled: settingsForm.value.aiAssessEnabled,
      aiTimeoutSeconds: settingsForm.value.aiTimeoutSeconds as number,
    }
    applySettings(await updateAdminDifficultySettings(input))
    toast.add({ severity: 'success', summary: '设置已保存', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: e instanceof ApiError ? e.message : '保存难度设置失败，请稍后再试',
      life: 4000,
    })
  } finally {
    settingsSaving.value = false
  }
}

function confirmRecalculate() {
  confirm.require({
    header: '立即重算题目难度',
    message: '将按当前公式立即重算全站自动调整的题目难度，确定继续？',
    icon: 'pi pi-refresh',
    acceptLabel: '立即重算',
    rejectLabel: '取消',
    accept: () => {
      void executeRun(difficultyJobKey, '全站题目难度重算')
    },
  })
}

// --- Run history ------------------------------------------------------------

const runs = ref<JobRunView[]>([])
const runsTotal = ref(0)
const runsPage = ref(1)
const runsLoading = ref(false)
const runsError = ref('')
const runsLoaded = ref(false)
const runJobFilter = ref('')
const runStatusFilter = ref('')

let runsSeq = 0

async function loadRuns(silent = false): Promise<boolean> {
  const seq = ++runsSeq
  if (!silent) {
    runsLoading.value = true
    runsError.value = ''
  }
  try {
    const result = await listAdminJobRuns({
      jobKey: runJobFilter.value || undefined,
      status: runStatusFilter.value || undefined,
      page: runsPage.value,
      pageSize: PAGE_SIZE,
    })
    if (seq !== runsSeq) return true
    runs.value = result.items
    runsTotal.value = result.total
    runsLoaded.value = true
    return true
  } catch (e) {
    if (seq !== runsSeq) return true
    if (!silent) {
      runsError.value = e instanceof ApiError ? e.message : '加载运行历史失败，请稍后再试'
    }
    return false
  } finally {
    if (seq === runsSeq && !silent) runsLoading.value = false
  }
}

function openHistory(job: JobView) {
  runJobFilter.value = job.key
  runStatusFilter.value = ''
  runsPage.value = 1
  activeTab.value = 'history'
  void loadRuns()
}

function onRunFilterChange() {
  runsPage.value = 1
  if (activeTab.value === 'history') {
    runsLoaded.value = false
    void loadRuns()
  }
}

function onJobFilterChange(value: string) {
  runJobFilter.value = value
  onRunFilterChange()
}

function onStatusFilterChange(value: string) {
  runStatusFilter.value = value
  onRunFilterChange()
}

function onRunsPage(event: { page: number }) {
  runsPage.value = event.page + 1
  void loadRuns()
}

watch(activeTab, (value) => {
  if (value === 'difficulty' && !settings.value && !settingsLoading.value) {
    void loadSettings()
  }
  if (value === 'history' && !runsLoading.value) {
    void loadRuns()
  }
})

onMounted(async () => {
  await loadJobs()
  if (!unmounted) schedulePoll()
})

onBeforeUnmount(() => {
  unmounted = true
  clearPoll()
})
</script>

<template>
  <div class="admin-jobs">
    <AppPageHeader
      eyebrow="ADMIN // JOBS"
      title="定时任务"
      description="管理全站定时任务、运行历史和题目难度公式；任务在后台异步执行，可随时查看进度。"
    />

    <AppSurface padding="none" class="admin-jobs__surface">
      <Tabs v-model:value="activeTab" lazy>
        <TabList>
          <Tab value="jobs">任务列表</Tab>
          <Tab value="difficulty">题目难度</Tab>
          <Tab value="history">运行历史</Tab>
        </TabList>

        <TabPanels>
          <TabPanel value="jobs">
            <div class="jobs-panel">
              <AppErrorState
                v-if="jobsError && jobs.length === 0"
                :description="jobsError"
                @retry="loadJobs()"
              />

              <AppLoadingState
                v-else-if="jobsLoading && jobs.length === 0"
                label="正在加载定时任务"
              />

              <template v-else>
                <Message
                  v-if="pollError"
                  severity="warn"
                  :closable="false"
                  role="status"
                  class="poll-error"
                >
                  <span class="poll-error__text">{{ pollError }}</span>
                  <Button
                    label="重试"
                    icon="pi pi-refresh"
                    size="small"
                    text
                    type="button"
                    @click="retryPoll"
                  />
                </Message>

                <div v-if="latestRun" class="latest-run" data-test="latest-run" role="status">
                  <div class="latest-run__head">
                    <span class="latest-run__title">
                      <i class="pi pi-play" aria-hidden="true" />
                      运行 <strong>#{{ latestRun.id }}</strong> · {{ jobName(latestRun.jobKey) }} ·
                      {{ statusLabel(latestRun.status) }}
                    </span>
                    <Tag
                      :value="statusLabel(latestRun.status)"
                      :severity="statusSeverity(latestRun.status)"
                    />
                  </div>
                  <ProgressBar
                    v-if="latestRun.status === 'running' && latestRun.progressTotal > 0"
                    :value="progressPercent(latestRun)"
                    :show-value="false"
                    class="latest-run__progress"
                  />
                </div>

                <Message v-if="jobsError" severity="warn" :closable="false" role="status">
                  {{ jobsError }} 已展示上一次成功加载的结果。
                </Message>

                <div class="job-grid">
                  <AppSurface
                    v-for="job in jobs"
                    :key="job.key"
                    padding="lg"
                    class="job-card"
                    :data-test="`job-card-${job.key}`"
                  >
                    <div class="job-card__head">
                      <div class="job-card__title-wrap">
                        <h2 class="job-card__title">{{ job.name }}</h2>
                        <p class="job-card__key">{{ job.key }}</p>
                      </div>
                      <Tag
                        :value="job.enabled ? '已启用' : '已停用'"
                        :severity="job.enabled ? 'success' : 'secondary'"
                      />
                    </div>

                    <p class="job-card__description">{{ job.description || '暂无描述' }}</p>

                    <dl class="job-meta">
                      <div class="job-meta__item">
                        <dt>调度</dt>
                        <dd>
                          <code>{{ job.cronExpr }}</code>
                        </dd>
                      </div>
                      <div class="job-meta__item">
                        <dt>时区</dt>
                        <dd>{{ job.timezone }}</dd>
                      </div>
                      <div class="job-meta__item">
                        <dt>下次运行</dt>
                        <dd>{{ formatDateTime(job.nextRunAt) }}</dd>
                      </div>
                      <div class="job-meta__item">
                        <dt>最近运行</dt>
                        <dd class="job-meta__status">
                          <Tag
                            :value="statusLabel(job.lastStatus)"
                            :severity="statusSeverity(job.lastStatus)"
                          />
                          <span>{{ formatDateTime(job.lastRunAt) }}</span>
                        </dd>
                      </div>
                    </dl>

                    <p v-if="job.lastError" class="job-card__error">{{ job.lastError }}</p>

                    <div v-if="job.activeRun" class="job-card__active" role="status">
                      <span>
                        当前运行 <strong>#{{ job.activeRun.id }}</strong> ·
                        {{ statusLabel(job.activeRun.status) }}
                      </span>
                      <span v-if="job.activeRun.progressTotal > 0" class="job-card__progress-text">
                        {{ progressText(job.activeRun) }}
                      </span>
                      <ProgressBar
                        v-if="job.activeRun.status === 'running' && job.activeRun.progressTotal > 0"
                        :value="progressPercent(job.activeRun)"
                        :show-value="false"
                      />
                    </div>

                    <div class="job-card__actions">
                      <Button
                        label="立即执行"
                        icon="pi pi-play"
                        size="small"
                        :loading="runningKey === job.key"
                        :disabled="job.activeRun !== null"
                        @click="confirmRun(job)"
                      />
                      <Button
                        label="编辑计划"
                        icon="pi pi-pencil"
                        size="small"
                        severity="secondary"
                        outlined
                        @click="openEdit(job)"
                      />
                      <Button
                        label="运行历史"
                        icon="pi pi-history"
                        size="small"
                        severity="secondary"
                        text
                        @click="openHistory(job)"
                      />
                      <Button
                        v-if="isDifficultyJob(job.key)"
                        label="难度设置"
                        icon="pi pi-sliders-h"
                        size="small"
                        severity="secondary"
                        text
                        @click="activeTab = 'difficulty'"
                      />
                    </div>
                  </AppSurface>

                  <AppSurface
                    v-if="ratingPlaceholderVisible"
                    padding="lg"
                    class="job-card job-card--planned"
                    data-test="job-rating-placeholder"
                  >
                    <div class="job-card__head">
                      <div class="job-card__title-wrap">
                        <h2 class="job-card__title">全站用户 Rating 重算</h2>
                        <p class="job-card__key">{{ ratingJobKey }}</p>
                      </div>
                      <Tag value="规划中" severity="secondary" />
                    </div>
                    <p class="job-card__description">
                      后续接入全站用户 Rating 评分重算，复用同一套调度、运行历史与进度展示。
                    </p>
                    <div class="job-card__actions">
                      <Button label="立即执行" icon="pi pi-play" size="small" disabled />
                    </div>
                  </AppSurface>
                </div>
              </template>
            </div>
          </TabPanel>

          <TabPanel value="difficulty">
            <div class="difficulty-panel">
              <AppErrorState
                v-if="settingsError && !settings"
                :description="settingsError"
                @retry="loadSettings"
              />

              <AppLoadingState v-else-if="settingsLoading && !settings" label="正在加载难度设置" />

              <template v-else>
                <Message v-if="settings && !settings.enabled" severity="warn" :closable="false">
                  难度自动调整已关闭：定时任务不会调整题目难度，立即重算也会被拒绝。
                </Message>

                <form class="difficulty-form" @submit.prevent="saveSettings">
                  <div class="settings-grid">
                    <AppFormField label="启用每日自动调整">
                      <div class="toggle-row">
                        <ToggleSwitch
                          v-model="settingsForm.enabled"
                          input-id="difficulty-enabled"
                        />
                        <label for="difficulty-enabled">{{
                          settingsForm.enabled ? '已启用' : '已关闭'
                        }}</label>
                      </div>
                    </AppFormField>

                    <AppFormField label="默认难度分" :error="settingsErrors.defaultScore">
                      <InputNumber
                        v-model="settingsForm.defaultScore"
                        input-id="difficulty-default-score"
                        :min="1"
                        :max="100"
                        :step="1"
                        :min-fraction-digits="0"
                        :max-fraction-digits="0"
                        fluid
                      />
                    </AppFormField>

                    <AppFormField label="最少样本数" :error="settingsErrors.minSample">
                      <InputNumber
                        v-model="settingsForm.minSample"
                        input-id="difficulty-min-sample"
                        :min="1"
                        :max="1000"
                        :step="1"
                        :min-fraction-digits="0"
                        :max-fraction-digits="0"
                        fluid
                      />
                    </AppFormField>

                    <AppFormField label="先验强度" :error="settingsErrors.priorStrength">
                      <InputNumber
                        v-model="settingsForm.priorStrength"
                        input-id="difficulty-prior-strength"
                        :min="1"
                        :max="100"
                        :step="1"
                        :min-fraction-digits="0"
                        :max-fraction-digits="0"
                        fluid
                      />
                    </AppFormField>

                    <AppFormField label="每日最大步长" :error="settingsErrors.maxStep">
                      <InputNumber
                        v-model="settingsForm.maxStep"
                        input-id="difficulty-max-step"
                        :min="1"
                        :max="100"
                        :step="1"
                        :min-fraction-digits="0"
                        :max-fraction-digits="0"
                        fluid
                      />
                    </AppFormField>

                    <AppFormField label="统计窗口天数" :error="settingsErrors.windowDays">
                      <InputNumber
                        v-model="settingsForm.windowDays"
                        input-id="difficulty-window-days"
                        :min="0"
                        :max="3650"
                        :step="1"
                        :min-fraction-digits="0"
                        :max-fraction-digits="0"
                        fluid
                      />
                    </AppFormField>

                    <AppFormField label="curve gamma" :error="settingsErrors.curveGamma">
                      <InputNumber
                        v-model="settingsForm.curveGamma"
                        input-id="difficulty-curve-gamma"
                        :min="1"
                        :max="3"
                        :step="0.1"
                        :min-fraction-digits="1"
                        :max-fraction-digits="2"
                        fluid
                      />
                    </AppFormField>

                    <AppFormField label="AI 超时秒数" :error="settingsErrors.aiTimeoutSeconds">
                      <InputNumber
                        v-model="settingsForm.aiTimeoutSeconds"
                        input-id="difficulty-ai-timeout"
                        :min="5"
                        :max="60"
                        :step="1"
                        :min-fraction-digits="0"
                        :max-fraction-digits="0"
                        fluid
                      />
                    </AppFormField>

                    <AppFormField label="包含比赛提交">
                      <div class="toggle-row">
                        <ToggleSwitch
                          v-model="settingsForm.includeContest"
                          input-id="difficulty-include-contest"
                        />
                        <label for="difficulty-include-contest">{{
                          settingsForm.includeContest ? '已包含' : '已排除'
                        }}</label>
                      </div>
                    </AppFormField>

                    <AppFormField label="AI 初评">
                      <div class="toggle-row">
                        <ToggleSwitch
                          v-model="settingsForm.aiAssessEnabled"
                          input-id="difficulty-ai-assess"
                        />
                        <label for="difficulty-ai-assess">{{
                          settingsForm.aiAssessEnabled ? '已启用' : '已关闭'
                        }}</label>
                      </div>
                    </AppFormField>
                  </div>

                  <div class="difficulty-actions">
                    <Button
                      type="submit"
                      label="保存设置"
                      icon="pi pi-check"
                      :loading="settingsSaving"
                    />
                    <Button
                      type="button"
                      label="立即重算"
                      icon="pi pi-refresh"
                      severity="warn"
                      outlined
                      :loading="difficultyRunning"
                      @click="confirmRecalculate"
                    />
                  </div>
                </form>

                <section class="formula-block" aria-label="难度公式说明">
                  <h2 class="formula-block__title">公式说明</h2>
                  <p class="formula-block__intro">
                    每日按统计窗口内的去重用户通过率计算目标难度；手动锁定的题目不会被自动调整，样本不足时跳过。
                  </p>
                  <pre class="formula-block__code">
priorPassRate = 1 − (prior − 1) / 99
smoothedPassRate = (accepted + priorStrength × priorPassRate) / (samples + priorStrength)
target = round(1 + 99 × (1 − smoothedPassRate)^curveGamma)
nextScore = clamp(current ± maxStep, 1, 100)</pre>
                  <p class="formula-block__note">
                    示例：先验 50、10 个样本、6 人通过、先验强度 10、gamma 1.5 时，目标难度约为 31
                    分。手动“立即重算”会直接移动到目标分，不受每日步长限制。
                  </p>
                </section>
              </template>
            </div>
          </TabPanel>

          <TabPanel value="history">
            <div class="history-panel">
              <div class="history-toolbar">
                <Select
                  :model-value="runJobFilter"
                  :options="jobOptions"
                  option-label="label"
                  option-value="value"
                  placeholder="全部任务"
                  aria-label="按任务筛选"
                  class="history-toolbar__select"
                  @update:model-value="onJobFilterChange"
                />
                <Select
                  :model-value="runStatusFilter"
                  :options="STATUS_OPTIONS"
                  option-label="label"
                  option-value="value"
                  placeholder="全部状态"
                  aria-label="按状态筛选"
                  class="history-toolbar__select"
                  @update:model-value="onStatusFilterChange"
                />
                <Button
                  label="刷新"
                  icon="pi pi-refresh"
                  size="small"
                  severity="secondary"
                  outlined
                  :loading="runsLoading"
                  @click="loadRuns()"
                />
              </div>

              <AppErrorState
                v-if="runsError && runs.length === 0"
                :description="runsError"
                @retry="loadRuns()"
              />

              <AppLoadingState
                v-else-if="runsLoading && runs.length === 0"
                label="正在加载运行历史"
              />

              <template v-else>
                <Message v-if="runsError" severity="warn" :closable="false" role="status">
                  {{ runsError }} 已展示上一次成功加载的结果。
                </Message>

                <DataTable
                  :value="runs"
                  :loading="runsLoading"
                  data-key="id"
                  data-test="runs-table"
                  size="small"
                  responsive-layout="scroll"
                  class="runs-table"
                >
                  <Column header="任务" style="min-width: 180px">
                    <template #body="{ data }">
                      <div class="run-job">
                        <span class="run-job__name">{{ jobName(data.jobKey) }}</span>
                        <span class="run-job__key">{{ data.jobKey }}</span>
                      </div>
                    </template>
                  </Column>
                  <Column header="触发方式" style="width: 96px">
                    <template #body="{ data }">{{ triggerLabel(data.trigger) }}</template>
                  </Column>
                  <Column header="状态" style="width: 96px">
                    <template #body="{ data }">
                      <Tag
                        :value="statusLabel(data.status)"
                        :severity="statusSeverity(data.status)"
                      />
                    </template>
                  </Column>
                  <Column header="进度" style="min-width: 150px">
                    <template #body="{ data }">
                      <div class="run-progress">
                        <ProgressBar
                          v-if="data.status === 'running' && data.progressTotal > 0"
                          :value="progressPercent(data)"
                          :show-value="false"
                          class="run-progress__bar"
                        />
                        <span class="run-progress__text">{{ progressText(data) }}</span>
                      </div>
                    </template>
                  </Column>
                  <Column header="开始时间" style="min-width: 150px">
                    <template #body="{ data }">{{ formatDateTime(data.startedAt) }}</template>
                  </Column>
                  <Column header="结束时间" style="min-width: 150px">
                    <template #body="{ data }">{{ formatDateTime(data.finishedAt) }}</template>
                  </Column>
                  <Column header="耗时" style="width: 100px">
                    <template #body="{ data }">{{ durationText(data) }}</template>
                  </Column>
                  <Column header="统计" style="min-width: 180px">
                    <template #body="{ data }">
                      <span class="run-stats">{{ statsText(data.stats) }}</span>
                    </template>
                  </Column>
                  <Column header="错误" style="min-width: 160px">
                    <template #body="{ data }">
                      <span class="run-error">{{ data.errorMessage || '—' }}</span>
                    </template>
                  </Column>
                  <template #empty>
                    <AppEmptyState
                      icon="pi pi-clock"
                      title="暂无运行记录"
                      description="任务执行后会在这里显示运行状态、进度与统计。"
                    />
                  </template>
                </DataTable>

                <Paginator
                  v-if="runsTotal > PAGE_SIZE"
                  :rows="PAGE_SIZE"
                  :total-records="runsTotal"
                  :first="(runsPage - 1) * PAGE_SIZE"
                  :page-link-size="5"
                  template="PrevPageLink PageLinks NextPageLink"
                  class="history-pager"
                  @page="onRunsPage"
                />
              </template>
            </div>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </AppSurface>

    <Dialog
      v-model:visible="editVisible"
      header="编辑任务计划"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 520px)' }"
    >
      <div class="plan-dialog">
        <p v-if="editingJob" class="plan-dialog__job">
          <strong>{{ editingJob.name }}</strong>
          <span>{{ editingJob.key }}</span>
        </p>

        <AppFormField label="启用任务">
          <div class="toggle-row">
            <ToggleSwitch v-model="planForm.enabled" input-id="job-enabled" />
            <label for="job-enabled">{{ planForm.enabled ? '已启用' : '已停用' }}</label>
          </div>
        </AppFormField>

        <AppFormField label="快捷计划">
          <div class="preset-row">
            <Button
              v-for="preset in CRON_PRESETS"
              :key="preset.value"
              :label="preset.label"
              size="small"
              severity="secondary"
              outlined
              type="button"
              @click="applyCronPreset(preset.value)"
            />
            <Button
              label="自定义"
              size="small"
              severity="secondary"
              text
              type="button"
              @click="planError = ''"
            />
          </div>
        </AppFormField>

        <AppFormField label="cron 表达式" :error="planError" hint="标准 5 段：分 时 日 月 周">
          <InputText id="job-cron" v-model="planForm.cronExpr" placeholder="0 3 * * *" fluid />
        </AppFormField>

        <AppFormField label="时区" hint="IANA 时区名，默认 Asia/Shanghai">
          <InputText id="job-timezone" v-model="planForm.timezone" fluid />
        </AppFormField>
      </div>

      <template #footer>
        <Button label="取消" severity="secondary" text type="button" @click="editVisible = false" />
        <Button
          label="保存计划"
          icon="pi pi-check"
          type="button"
          :loading="savingPlan"
          @click="savePlan"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.admin-jobs {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
}

.admin-jobs__surface {
  overflow: hidden;
}

.jobs-panel,
.difficulty-panel,
.history-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  padding: var(--kapoj-space-4);
}

.job-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 440px), 1fr));
  gap: var(--kapoj-space-4);
}

.job-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.job-card--planned {
  opacity: 0.72;
  border-style: dashed;
}

.job-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.job-card__title-wrap {
  min-width: 0;
}

.job-card__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.05rem;
  line-height: 1.3;
}

.job-card__key,
.run-job__key {
  margin: 2px 0 0;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  overflow-wrap: anywhere;
}

.job-card__description {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.85rem;
  line-height: 1.6;
}

.job-meta {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-3);
  margin: 0;
  background: var(--kapoj-surface-raised);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.job-meta__item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.job-meta__item dt {
  color: var(--kapoj-text-muted);
  font-size: 11px;
  font-weight: 600;
}

.job-meta__item dd {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.job-meta__status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.job-card__error {
  padding: var(--kapoj-space-2) var(--kapoj-space-3);
  margin: 0;
  color: var(--kapoj-danger);
  font-size: 12px;
  background: color-mix(in srgb, var(--kapoj-danger) 8%, transparent);
  border-radius: var(--kapoj-radius-sm);
}

.job-card__active,
.latest-run {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 25%, transparent);
  border-radius: var(--kapoj-radius-sm);
}

.job-card__progress-text {
  font-family: var(--kapoj-font-mono);
  color: var(--kapoj-text);
}

.job-card__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  margin-top: auto;
}

.poll-error__text {
  flex: 1 1 auto;
  min-width: 0;
}

.latest-run {
  margin-bottom: var(--kapoj-space-1);
}

.latest-run__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.latest-run__title {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  color: var(--kapoj-text);
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
  gap: var(--kapoj-space-4);
}

.difficulty-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  min-height: 40px;
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.difficulty-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.formula-block {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-4);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.formula-block__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.95rem;
}

.formula-block__intro,
.formula-block__note {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.83rem;
  line-height: 1.6;
}

.formula-block__code {
  padding: var(--kapoj-space-3);
  margin: 0;
  overflow-x: auto;
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  line-height: 1.7;
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.history-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.history-toolbar__select {
  min-width: 180px;
}

.run-job {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.run-job__name {
  color: var(--kapoj-text);
  font-weight: 600;
}

.run-progress {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.run-progress__bar {
  height: 6px;
}

.run-progress__text {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.run-stats {
  display: block;
  max-width: 280px;
  overflow: hidden;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.run-error {
  display: block;
  max-width: 320px;
  color: var(--kapoj-danger);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.history-pager {
  align-self: flex-end;
}

.plan-dialog {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.plan-dialog__job {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.plan-dialog__job strong {
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-sans, inherit);
  font-size: 0.95rem;
}

.preset-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

@media (max-width: 640px) {
  .jobs-panel,
  .difficulty-panel,
  .history-panel {
    padding: var(--kapoj-space-3);
  }

  .history-toolbar__select {
    flex: 1 1 100%;
  }
}
</style>
