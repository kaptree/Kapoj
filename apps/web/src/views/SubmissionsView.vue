<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'

import { ApiError } from '@/api/http'
import { listSubmissions } from '@/api/submissions'
import type { SubmissionLanguage, SubmissionListItem, SubmissionStatus } from '@/api/types'
import SubmissionDetailDrawer from '@/components/SubmissionDetailDrawer.vue'
import UserIdentityName from '@/components/UserIdentityName.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppStatusTag from '@/components/ui/AppStatusTag.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'
import { languageLabel, SUBMISSION_LANGUAGE_FILTERS, SUBMISSION_STATUSES } from '@/utils/submission'

const PAGE_SIZE = 20
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const canRejudge = computed(() => auth.can(PERM.submission.rejudge))

interface FilterState {
  problemId: number | null
  contestId: number | null
  userId: number | null
  language: SubmissionLanguage | '' | null
  status: SubmissionStatus | '' | null
  nickname: string
}

function emptyFilters(): FilterState {
  return {
    problemId: null,
    contestId: null,
    userId: null,
    language: '',
    status: '',
    nickname: '',
  }
}

// Draft filters are bound to the form; applied filters come from the URL and
// are the only ones used by background SSE/pagination reloads.
const filters = ref<FilterState>(emptyFilters())
const applied = ref<FilterState & { page: number }>({ ...emptyFilters(), page: 1 })
const items = ref<SubmissionListItem[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')
const live = ref(false)

const drawer = ref(false)
const selectedSubmissionId = ref<number | null>(null)

const first = computed(() => (applied.value.page - 1) * PAGE_SIZE)

let latest = 0
let source: EventSource | null = null
let reloadTimer: number | undefined
let syncTimer: number | undefined

function firstValue(v: unknown): string {
  const raw = Array.isArray(v) ? v[0] : v
  return typeof raw === 'string' ? raw : ''
}

function numberQuery(v: unknown): number | null {
  const raw = firstValue(v)
  if (raw === '') return null
  const n = Number(raw)
  return Number.isInteger(n) && n >= 0 ? n : null
}

function applyQuery() {
  const parsed: FilterState = {
    problemId: numberQuery(route.query.problemId),
    contestId: numberQuery(route.query.contestId),
    userId: numberQuery(route.query.userId),
    language: (firstValue(route.query.language) as SubmissionLanguage | '') || '',
    status: (firstValue(route.query.status) as SubmissionStatus | '') || '',
    nickname: firstValue(route.query.nickname),
  }
  Object.assign(filters.value, parsed)
  Object.assign(applied.value, parsed)
  applied.value.page = numberQuery(route.query.page) ?? 1
}

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listSubmissions({
      problemId: applied.value.problemId ?? undefined,
      contestId: applied.value.contestId ?? undefined,
      userId: applied.value.userId ?? undefined,
      language: applied.value.language || undefined,
      status: applied.value.status || undefined,
      nickname: applied.value.nickname,
      page: applied.value.page,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latest) return
    items.value = data.items
    total.value = data.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载测评记录失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function queryFrom(state: FilterState, nextPage: number): Record<string, string> {
  const q: Record<string, string> = {}
  if (state.problemId != null) q.problemId = String(state.problemId)
  if (state.contestId != null) q.contestId = String(state.contestId)
  if (state.userId != null) q.userId = String(state.userId)
  if (state.language) q.language = state.language
  if (state.status) q.status = state.status
  if (state.nickname.trim()) q.nickname = state.nickname.trim()
  if (nextPage > 1) q.page = String(nextPage)
  return q
}

function search() {
  void router.push({ name: 'submissions', query: queryFrom(filters.value, 1) })
}

function reset() {
  if (Object.keys(route.query).length === 0) {
    void load()
    return
  }
  void router.push({ name: 'submissions', query: {} })
}

function changePage(next: number) {
  void router.push({ name: 'submissions', query: queryFrom(applied.value, next) })
}

function onPage(event: { page: number }) {
  changePage(event.page + 1)
}

watch(
  () => route.query,
  () => {
    applyQuery()
    void load()
  },
  { immediate: true },
)

function onRowClick(event: { data: SubmissionListItem }) {
  selectedSubmissionId.value = event.data.id
  drawer.value = true
}

function scheduleReload() {
  if (reloadTimer !== undefined) return
  reloadTimer = window.setTimeout(() => {
    reloadTimer = undefined
    void load()
  }, 300)
}

function connectStream() {
  if (typeof EventSource === 'undefined') return
  source = new EventSource('/api/submissions/stream')
  source.onopen = () => {
    live.value = true
    scheduleReload()
  }
  source.onerror = () => {
    live.value = false
  }
  source.onmessage = () => scheduleReload()
}

onMounted(() => {
  connectStream()
  syncTimer = window.setInterval(() => void load(), 30000)
})
onBeforeUnmount(() => {
  source?.close()
  if (reloadTimer !== undefined) window.clearTimeout(reloadTimer)
  if (syncTimer !== undefined) window.clearInterval(syncTimer)
})
</script>

<template>
  <div class="submissions-page">
    <AppPageHeader
      eyebrow="SUBMISSIONS // LIVE"
      title="测评记录"
      description="查看全平台提交状态、耗时与内存；支持实时刷新。"
    >
      <template #actions>
        <span class="live-status" :class="{ 'is-live': live }" role="status" aria-live="polite">
          <span class="live-dot" aria-hidden="true" />
          {{ live ? '实时刷新' : '连接中断，正在重连' }}
        </span>
        <Button
          label="刷新"
          icon="pi pi-refresh"
          severity="secondary"
          outlined
          :loading="loading"
          @click="load"
        />
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="submissions-surface">
      <form class="filter-bar" role="search" @submit.prevent="search">
        <div class="filter-grid">
          <div class="filter-field">
            <label for="filter-problem">题目 ID</label>
            <InputNumber
              v-model="filters.problemId"
              input-id="filter-problem"
              :min="0"
              :show-buttons="false"
              :use-grouping="false"
              fluid
            />
          </div>
          <div class="filter-field">
            <label for="filter-contest">比赛 ID</label>
            <InputNumber
              v-model="filters.contestId"
              input-id="filter-contest"
              :min="0"
              :show-buttons="false"
              :use-grouping="false"
              fluid
            />
          </div>
          <div class="filter-field">
            <label for="filter-user">用户 ID</label>
            <InputNumber
              v-model="filters.userId"
              input-id="filter-user"
              :min="0"
              :show-buttons="false"
              :use-grouping="false"
              fluid
            />
          </div>
          <div class="filter-field">
            <label for="filter-nickname">昵称</label>
            <InputText id="filter-nickname" v-model="filters.nickname" fluid />
          </div>
          <div class="filter-field">
            <label for="filter-language">语言</label>
            <Select
              v-model="filters.language"
              input-id="filter-language"
              :options="SUBMISSION_LANGUAGE_FILTERS"
              option-label="label"
              option-value="value"
              show-clear
              fluid
            />
          </div>
          <div class="filter-field">
            <label for="filter-status">状态</label>
            <Select
              v-model="filters.status"
              input-id="filter-status"
              :options="SUBMISSION_STATUSES"
              option-label="label"
              option-value="value"
              show-clear
              fluid
            />
          </div>
        </div>
        <div class="filter-actions">
          <Button type="submit" label="筛选" icon="pi pi-filter" />
          <Button type="button" label="重置" severity="secondary" text @click="reset" />
        </div>
      </form>

      <AppErrorState v-if="error" :description="error" @retry="load" />

      <div class="result-bar">
        <span>共 {{ total }} 条记录</span>
      </div>

      <AppLoadingState v-if="loading && items.length === 0" label="正在加载测评记录" />
      <DataTable
        v-else-if="!error || items.length > 0"
        :value="items"
        :loading="loading"
        data-key="id"
        size="small"
        row-hover
        responsive-layout="scroll"
        class="submissions-table"
        @row-click="onRowClick"
      >
        <Column header="#" style="width: 50px">
          <template #body="{ data }">
            <span class="num">{{ data.id }}</span>
          </template>
        </Column>
        <Column header="提交者" style="width: 120px">
          <template #body="{ data }">
            <div class="user-cell">
              <UserIdentityName
                :nickname="data.nickname"
                :username="data.username"
                :title="data.equippedTitle"
                :badges="data.equippedBadges"
                size="small"
                :link-to="{ name: 'user-profile', params: { id: data.userId } }"
              />
            </div>
          </template>
        </Column>
        <Column header="题目" style="min-width: 150px">
          <template #body="{ data }">
            <RouterLink
              :to="{ name: 'problem-detail', params: { id: data.problemId } }"
              class="problem-link"
              @click.stop
            >
              {{ data.problemTitle }}
            </RouterLink>
          </template>
        </Column>
        <Column header="状态" style="width: 95px">
          <template #body="{ data }">
            <AppStatusTag :status="data.status" />
          </template>
        </Column>
        <Column header="语言" style="width: 75px">
          <template #body="{ data }">{{ languageLabel(data.language) }}</template>
        </Column>
        <Column header="耗时" style="width: 75px">
          <template #body="{ data }">
            <span class="num">{{
              data.language === 'answer' ? '—' : `${data.timeUsedMs} ms`
            }}</span>
          </template>
        </Column>
        <Column header="内存" style="width: 80px">
          <template #body="{ data }">
            <span class="num">{{
              data.language === 'answer' ? '—' : `${data.memoryUsedKb} KB`
            }}</span>
          </template>
        </Column>
        <Column header="得分" style="width: 50px">
          <template #body="{ data }"
            ><span class="num">{{ data.score }}</span></template
          >
        </Column>
        <Column header="提交时间" style="min-width: 140px">
          <template #body="{ data }">{{ formatDateTime(data.createdAt) }}</template>
        </Column>
        <template #empty>
          <AppEmptyState
            icon="pi pi-inbox"
            title="暂无测评记录"
            description="调整筛选条件，或等待新的提交。"
          />
        </template>
      </DataTable>

      <Paginator
        v-if="total > 0"
        :rows="PAGE_SIZE"
        :total-records="total"
        :first="first"
        :page-link-size="5"
        template="PrevPageLink PageLinks NextPageLink"
        class="pager"
        @page="onPage"
      />
    </AppSurface>

    <SubmissionDetailDrawer
      v-model:visible="drawer"
      :submission-id="selectedSubmissionId"
      :can-rejudge="canRejudge"
      @rejudged="load"
    />
  </div>
</template>

<style scoped>
.submissions-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.submissions-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.live-status {
  display: inline-flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding: 3px var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-size: 11px;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}

.live-status.is-live {
  color: var(--kapoj-success);
  border-color: color-mix(in srgb, var(--kapoj-success) 32%, transparent);
}

.live-dot {
  width: 6px;
  height: 6px;
  background: currentColor;
  border-radius: 50%;
}

.live-status.is-live .live-dot {
  box-shadow: 0 0 8px var(--kapoj-success);
  animation: live-pulse 1.6s ease-in-out infinite;
}

.filter-bar {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.filter-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--kapoj-space-2);
}

.filter-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.filter-field label {
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.filter-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
}

.result-bar {
  display: flex;
  justify-content: flex-end;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.submissions-table,
.judge-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.submissions-table :deep(.p-datatable-table) {
  min-width: 840px;
}

.submissions-table :deep(.p-datatable-thead > tr > th),
.judge-table :deep(.p-datatable-thead > tr > th) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  background: var(--kapoj-bg-elevated);
  border-color: var(--kapoj-border);
}

.submissions-table :deep(.p-datatable-tbody > tr > td),
.judge-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.submissions-table :deep(.p-datatable-tbody > tr) {
  cursor: pointer;
}

.user-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  min-width: 0;
}

.problem-link {
  color: var(--kapoj-text);
  font-weight: 600;
}

.problem-link:hover {
  color: var(--kapoj-primary);
}

.pager {
  align-self: flex-end;
}

@keyframes live-pulse {
  0%,
  100% {
    opacity: 1;
  }

  50% {
    opacity: 0.35;
  }
}

@media (max-width: 700px) {
  .filter-actions {
    width: 100%;
  }

  .filter-actions :deep(.p-button) {
    flex: 1 1 auto;
  }

  .pager {
    align-self: stretch;
  }
}

@media (prefers-reduced-motion: reduce) {
  .live-status.is-live .live-dot {
    animation: none;
  }
}
</style>
