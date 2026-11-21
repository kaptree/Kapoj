<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import Tag from 'primevue/tag'

import { getAgent, getAgentRatingHistory } from '@/api/agents'
import { ApiError } from '@/api/http'
import type { AgentRatingKind, AgentRatingRow, AgentView } from '@/api/types'
import MarkdownView from '@/components/MarkdownView.vue'
import SubmissionDetailDrawer from '@/components/SubmissionDetailDrawer.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import RatingLevelName from '@/components/RatingLevelName.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useTheme } from '@/theme/theme'
import { formatDateTime } from '@/utils/format'

const HISTORY_PAGE_SIZE = 10
const { isDark } = useTheme()

const KIND_META: Record<AgentRatingKind, { label: string; severity: string }> = {
  solve: { label: '通过', severity: 'success' },
  abandon: { label: '放弃', severity: 'danger' },
  timeout: { label: '超时', severity: 'warn' },
  exhausted: { label: '次数耗尽', severity: 'danger' },
  reset: { label: '重置', severity: 'info' },
}

const route = useRoute()

const agent = ref<AgentView | null>(null)
const loading = ref(true)
const error = ref('')

const history = ref<AgentRatingRow[]>([])
const historyTotal = ref(0)
const historyPage = ref(1)
const historyLoading = ref(false)
const historyError = ref('')
const submissionDrawer = ref(false)
const selectedSubmissionId = ref<number | null>(null)

function openSubmission(id: number) {
  selectedSubmissionId.value = id
  submissionDrawer.value = true
}

const historyFirst = computed(() => (historyPage.value - 1) * HISTORY_PAGE_SIZE)
const statusLabel = computed(() => (agent.value?.status === 'disabled' ? '已停用' : '正常'))
const statusSeverity = computed(() => (agent.value?.status === 'disabled' ? 'danger' : 'success'))

let detailRequest = 0
let historyRequest = 0

function currentId(): string {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
}

function isNotFound(e: unknown): boolean {
  return e instanceof ApiError && e.code === 40400
}

async function loadAgent() {
  const seq = ++detailRequest
  loading.value = true
  error.value = ''
  try {
    const data = await getAgent(currentId())
    if (seq !== detailRequest) return
    agent.value = data.agent
  } catch (e) {
    if (seq !== detailRequest) return
    agent.value = null
    error.value = isNotFound(e) ? 'Agent 不存在或已删除' : '加载 Agent 详情失败，请稍后再试'
  } finally {
    if (seq === detailRequest) loading.value = false
  }
}

async function loadHistory() {
  const seq = ++historyRequest
  historyLoading.value = true
  historyError.value = ''
  try {
    const data = await getAgentRatingHistory(currentId(), {
      page: historyPage.value,
      pageSize: HISTORY_PAGE_SIZE,
    })
    if (seq !== historyRequest) return
    history.value = data.items
    historyTotal.value = data.total
  } catch {
    if (seq !== historyRequest) return
    history.value = []
    historyTotal.value = 0
    historyError.value = '加载 Rating 历史失败，请稍后再试'
  } finally {
    if (seq === historyRequest) historyLoading.value = false
  }
}

function retry() {
  void loadAgent()
  void loadHistory()
}

function onHistoryPage(event: { page: number }) {
  historyPage.value = event.page + 1
  void loadHistory()
}

function formatSolveTime(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '—'
  const totalSeconds = Math.floor(ms / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const clock = [minutes, seconds].map((n) => String(n).padStart(2, '0')).join(':')
  return hours > 0 ? `${hours}:${clock}` : clock
}

function historyTitle(row: AgentRatingRow): string {
  if (row.kind === 'reset') return 'Rating 重置'
  return row.problemTitle || '题目已删除'
}

function kindLabel(kind: AgentRatingKind): string {
  return KIND_META[kind].label
}

function kindSeverity(kind: AgentRatingKind): string {
  return KIND_META[kind].severity
}

function deltaLabel(delta: number): string {
  return delta > 0 ? `+${delta}` : String(delta)
}

function deltaClass(delta: number): string {
  if (delta > 0) return 'delta delta--positive'
  if (delta < 0) return 'delta delta--negative'
  return 'delta'
}

watch(
  () => route.params.id,
  () => {
    historyPage.value = 1
    history.value = []
    historyTotal.value = 0
    historyError.value = ''
    void loadAgent()
    void loadHistory()
  },
  { immediate: true },
)
</script>

<template>
  <div class="agent-detail-page">
    <AppPageHeader
      eyebrow="AGENTS // PROFILE"
      title="Agent 详情"
      description="公开的 Agent 资料、独立 Rating 与答题表现。"
    >
      <template #actions>
        <RouterLink class="back-link" :to="{ name: 'agents' }">
          <i class="pi pi-arrow-left" aria-hidden="true" />
          返回 Agent 榜单
        </RouterLink>
      </template>
    </AppPageHeader>

    <AppSurface v-if="loading" padding="lg">
      <AppLoadingState label="正在加载 Agent 详情" />
    </AppSurface>

    <AppSurface v-else-if="error" padding="lg">
      <AppErrorState :description="error" data-test="detail-error" @retry="retry" />
    </AppSurface>

    <template v-else-if="agent">
      <AppSurface as="section" tone="raised" padding="lg" class="agent-profile">
        <header class="profile-head">
          <div class="profile-titles">
            <h2 class="profile-name" data-test="agent-name">
              <RatingLevelName :name="agent.name" :level="agent.level" inherit-typography />
            </h2>
            <p class="profile-model">
              <i class="pi pi-microchip-ai" aria-hidden="true" />
              <span>{{ agent.modelName }}</span>
            </p>
          </div>
          <Tag :value="statusLabel" :severity="statusSeverity" data-test="agent-status" />
        </header>

        <div v-if="agent.modelIntro" class="profile-intro" data-test="agent-intro">
          <MarkdownView :source="agent.modelIntro" :theme="isDark ? 'dark' : 'light'" />
        </div>

        <p class="profile-author" data-test="agent-author">
          作者：<RatingLevelName :name="agent.author.nickname || agent.author.username" :level="agent.author.level" />
          <span class="author-username">@{{ agent.author.username }}</span>
        </p>

        <dl class="stat-grid">
          <div class="stat">
            <dt>Rating</dt>
            <dd class="num" data-test="agent-rating">{{ agent.rating }}</dd>
          </div>
          <div class="stat">
            <dt>通过题数</dt>
            <dd class="num" data-test="agent-passed">{{ agent.passedCount }}</dd>
          </div>
          <div class="stat">
            <dt>尝试题数</dt>
            <dd class="num" data-test="agent-attempted">{{ agent.attemptedCount }}</dd>
          </div>
          <div class="stat">
            <dt>平均用时</dt>
            <dd class="num" data-test="agent-avg-time">
              {{ formatSolveTime(agent.avgSolveTimeMs) }}
            </dd>
          </div>
        </dl>
      </AppSurface>

      <AppSurface as="section" padding="lg" class="history-surface">
        <header class="history-head">
          <h2>Rating 历史</h2>
          <p>按时间倒序展示每次通过、放弃、超时与重置带来的 Rating 变化。</p>
        </header>

        <div v-if="historyError" class="history-error">
          <Message severity="error" :closable="false" role="alert">
            {{ historyError }}
          </Message>
          <Button
            type="button"
            label="重试"
            icon="pi pi-refresh"
            severity="secondary"
            outlined
            size="small"
            data-test="history-retry"
            @click="loadHistory"
          />
        </div>

        <AppLoadingState
          v-if="historyLoading && history.length === 0"
          label="正在加载 Rating 历史"
        />

        <AppEmptyState
          v-else-if="history.length === 0 && !historyError"
          icon="pi pi-chart-line"
          title="暂无 Rating 记录"
          description="Agent 通过、放弃或超时题目后，Rating 变化会记录在这里。"
        />

        <template v-else-if="history.length > 0">
          <DataTable
            :value="history"
            :loading="historyLoading"
            data-key="id"
            size="small"
            responsive-layout="scroll"
            class="history-table"
            data-test="agent-rating-history"
          >
            <Column header="题目" style="min-width: 180px">
              <template #body="{ data }">
                <div class="history-problem">
                  <strong>{{ historyTitle(data) }}</strong>
                  <span v-if="data.problemSlug" class="history-slug mono">
                    {{ data.problemSlug }}
                  </span>
                </div>
              </template>
            </Column>

            <Column header="类型" style="width: 110px">
              <template #body="{ data }">
                <Tag :value="kindLabel(data.kind)" :severity="kindSeverity(data.kind)" />
              </template>
            </Column>

            <Column header="难度" style="width: 80px">
              <template #body="{ data }">
                <span class="mono">{{ data.difficulty || '—' }}</span>
              </template>
            </Column>

            <Column header="提交" style="width: 90px">
              <template #body="{ data }">
                <Button
                  v-if="data.submissionId != null"
                  type="button"
                  label="查看"
                  size="small"
                  text
                  severity="secondary"
                  data-test="rating-submission-link"
                  @click="openSubmission(data.submissionId)"
                />
                <span v-else class="muted">—</span>
              </template>
            </Column>

            <Column header="变化" style="width: 90px">
              <template #body="{ data }">
                <span :class="deltaClass(data.delta)">{{ deltaLabel(data.delta) }}</span>
              </template>
            </Column>

            <Column header="Rating" style="min-width: 130px">
              <template #body="{ data }">
                <span class="mono">{{ data.ratingBefore }} → {{ data.ratingAfter }}</span>
              </template>
            </Column>

            <Column header="用时" style="width: 100px">
              <template #body="{ data }">
                <span class="mono">{{ formatSolveTime(data.solveTimeMs) }}</span>
              </template>
            </Column>

            <Column header="时间" style="min-width: 160px">
              <template #body="{ data }">
                <span class="muted">{{ formatDateTime(data.createdAt) }}</span>
              </template>
            </Column>
          </DataTable>

          <Paginator
            v-if="historyTotal > HISTORY_PAGE_SIZE"
            :rows="HISTORY_PAGE_SIZE"
            :total-records="historyTotal"
            :first="historyFirst"
            :page-link-size="5"
            template="PrevPageLink PageLinks NextPageLink"
            class="history-pager"
            @page="onHistoryPage"
          />
        </template>
      </AppSurface>
    </template>

    <SubmissionDetailDrawer
      v-model:visible="submissionDrawer"
      :submission-id="selectedSubmissionId"
      :can-rejudge="false"
    />
  </div>
</template>

<style scoped>
.agent-detail-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: var(--kapoj-content-max);
}

.back-link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  font-weight: 500;
  text-decoration: none;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
  transition:
    color var(--kapoj-transition),
    border-color var(--kapoj-transition);
}

.back-link:hover {
  color: var(--kapoj-primary);
  text-decoration: none;
  border-color: var(--kapoj-border-strong);
}

.agent-profile {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.profile-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.profile-titles {
  min-width: 0;
}

.profile-name {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.35rem;
  font-weight: 600;
  line-height: 1.2;
  overflow-wrap: anywhere;
}

.profile-model {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.profile-intro {
  max-width: 80ch;
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
  line-height: 1.6;
}

.profile-author {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.author-username {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: var(--kapoj-space-3);
  margin: 0;
}

.stat {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  min-width: 0;
  padding: var(--kapoj-space-3);
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.stat dt {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.stat dd {
  margin: 0;
  color: var(--kapoj-text);
  overflow-wrap: anywhere;
}

.num {
  font-family: var(--kapoj-font-mono);
  font-size: 1.05rem;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.history-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.history-head h2 {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.history-head p {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  line-height: 1.55;
}

.history-error {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.history-error :deep(.p-message) {
  flex: 1 1 240px;
}

.history-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.history-table :deep(.p-datatable-thead > tr > th) {
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

.history-table :deep(.p-datatable-tbody > tr > td) {
  padding: 10px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.history-problem {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.history-problem strong {
  color: var(--kapoj-text);
  font-size: 0.8125rem;
  overflow-wrap: anywhere;
}

.history-slug {
  color: var(--kapoj-text-muted);
}

.delta {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-weight: 600;
}

.delta--positive {
  color: var(--kapoj-success);
}

.delta--negative {
  color: var(--kapoj-danger);
}

.mono {
  overflow-wrap: anywhere;
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.history-pager {
  align-self: flex-end;
}

@media (max-width: 640px) {
  .agent-detail-page {
    gap: var(--kapoj-space-4);
  }

  .stat-grid {
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  }

  .back-link {
    width: 100%;
    justify-content: center;
  }

  .history-pager {
    align-self: stretch;
  }
}

@media (prefers-reduced-motion: reduce) {
  .back-link {
    transition: none;
  }
}
</style>
