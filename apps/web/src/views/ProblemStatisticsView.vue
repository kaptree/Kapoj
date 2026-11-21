<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Paginator from 'primevue/paginator'
import ProgressBar from 'primevue/progressbar'

import { ApiError } from '@/api/http'
import { getStatistics, getSubmissionStats } from '@/api/problems'
import type { ProblemStats, ProblemSubmissionMetric, SubmissionStatus } from '@/api/types'
import UserIdentityName from '@/components/UserIdentityName.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppStatusTag from '@/components/ui/AppStatusTag.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { resolveAssetUrl } from '@/utils/asset'
import { formatDateTime } from '@/utils/format'
import { languageLabel, statusLabel } from '@/utils/submission'

type MetricSort = 'time' | 'memory' | 'code_length' | 'created_at'

const route = useRoute()

const problemId = computed(() => {
  const id = route.params.id
  return typeof id === 'string' ? id : ''
})

const stats = ref<ProblemStats | null>(null)
const loading = ref(false)
const error = ref('')

const metrics = ref<ProblemSubmissionMetric[]>([])
const metricTotal = ref(0)
const metricsLoading = ref(false)
const metricsError = ref('')
const sort = ref<MetricSort>('created_at')
const order = ref<'asc' | 'desc'>('desc')
const page = ref(1)
const PAGE_SIZE = 20

const rate = computed(() => (stats.value ? `${(stats.value.acceptRate * 100).toFixed(1)}%` : '-'))
const acceptPercent = computed(() =>
  stats.value ? Math.min(100, Math.max(0, stats.value.acceptRate * 100)) : 0,
)

const STATUS_COLORS: Record<SubmissionStatus, string> = {
  accepted: 'var(--kapoj-status-ac)',
  wrong_answer: 'var(--kapoj-status-wa)',
  time_limit_exceeded: 'var(--kapoj-status-tle)',
  memory_limit_exceeded: 'var(--kapoj-status-mle)',
  runtime_error: 'var(--kapoj-status-re)',
  compile_error: 'var(--kapoj-status-ce)',
  system_error: 'var(--kapoj-status-se)',
  judging: 'var(--kapoj-status-judging)',
  pending: 'var(--kapoj-text-muted)',
}

const visibleCounts = computed(() =>
  (stats.value?.statusCounts ?? []).filter((item) => item.count > 0),
)
const totalVisible = computed(() => visibleCounts.value.reduce((sum, item) => sum + item.count, 0))
const maxCount = computed(() =>
  visibleCounts.value.reduce((max, item) => Math.max(max, item.count), 0),
)

const pieSlices = computed(() => {
  const circumference = 2 * Math.PI * 40
  let offset = 0
  return visibleCounts.value.map((item) => {
    const length = totalVisible.value > 0 ? (item.count / totalVisible.value) * circumference : 0
    const slice = {
      ...item,
      color: STATUS_COLORS[item.status] ?? 'var(--kapoj-text-muted)',
      length,
      offset,
    }
    offset += length
    return slice
  })
})

const chartSummary = computed(() => {
  if (totalVisible.value === 0) return '暂无可见提交'
  const parts = visibleCounts.value.map(
    (item) => `${statusLabel(item.status)} ${item.count} 条，占 ${percentLabel(item.percentage)}`,
  )
  return `各状态占比：${parts.join('；')}`
})

const sortOptions: { value: MetricSort; label: string }[] = [
  { value: 'created_at', label: '提交时间' },
  { value: 'time', label: '耗时' },
  { value: 'memory', label: '内存' },
  { value: 'code_length', label: '代码长度' },
]

let latestStats = 0
let latestMetrics = 0

function percentLabel(percentage: number): string {
  return `${(percentage * 100).toFixed(1)}%`
}

function barWidth(count: number): string {
  return maxCount.value > 0 ? `${(count / maxCount.value) * 100}%` : '0%'
}

function codeLengthLabel(length: number): string {
  return `${length} B`
}

function resolvedAvatar(url: string | undefined): string | undefined {
  return url ? resolveAssetUrl(url) : undefined
}

function statusColor(status: SubmissionStatus): string {
  return STATUS_COLORS[status] ?? 'var(--kapoj-text-muted)'
}

async function loadStats() {
  const seq = ++latestStats
  loading.value = true
  error.value = ''
  stats.value = null
  try {
    const data = await getStatistics(problemId.value)
    if (seq !== latestStats) return
    stats.value = data
  } catch (e) {
    if (seq !== latestStats) return
    error.value = e instanceof ApiError ? e.message : '加载统计失败，请稍后再试'
  } finally {
    if (seq === latestStats) loading.value = false
  }
}

async function loadMetrics() {
  const seq = ++latestMetrics
  metricsLoading.value = true
  metricsError.value = ''
  try {
    const data = await getSubmissionStats(problemId.value, {
      sort: sort.value,
      order: order.value,
      page: page.value,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latestMetrics) return
    metrics.value = data.items
    metricTotal.value = data.total
  } catch (e) {
    if (seq !== latestMetrics) return
    metrics.value = []
    metricTotal.value = 0
    metricsError.value = e instanceof ApiError ? e.message : '加载提交明细失败，请稍后再试'
  } finally {
    if (seq === latestMetrics) metricsLoading.value = false
  }
}

function reload() {
  void loadStats()
  void loadMetrics()
}

function onSortChange() {
  page.value = 1
  void loadMetrics()
}

function onOrderChange() {
  page.value = 1
  void loadMetrics()
}

function onPage(event: { page: number }) {
  page.value = event.page + 1
  void loadMetrics()
}

watch(sort, onSortChange)
watch(order, onOrderChange)
watch(
  () => route.params.id,
  () => {
    page.value = 1
    reload()
  },
  { immediate: true },
)
</script>

<template>
  <div class="statistics-page">
    <AppPageHeader
      eyebrow="PROBLEM // STATISTICS"
      title="提交统计"
      :description="`题目 #${problemId}`"
    >
      <template #actions>
        <Button
          as="router-link"
          :to="`/problems/${problemId}`"
          label="返回题目"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
        <Button
          as="router-link"
          :to="`/submissions?problemId=${problemId}`"
          label="查看提交记录"
          icon="pi pi-list"
        />
      </template>
    </AppPageHeader>

    <AppSurface padding="lg">
      <AppLoadingState v-if="loading && !stats" label="正在加载统计" :rows="2" />
      <AppErrorState v-else-if="error && !stats" :description="error" @retry="reload" />
      <AppEmptyState
        v-else-if="!stats"
        icon="pi pi-chart-bar"
        title="暂时无法获取统计"
        description="该题目暂时没有可用的提交统计。"
      />
      <div v-else class="stats">
        <dl class="stats-grid">
          <div>
            <dt>提交数</dt>
            <dd class="num">{{ stats.submitCount }}</dd>
          </div>
          <div>
            <dt>通过数</dt>
            <dd class="num">{{ stats.acceptCount }}</dd>
          </div>
          <div>
            <dt>通过率</dt>
            <dd class="num">{{ rate }}</dd>
          </div>
        </dl>
        <ProgressBar :value="acceptPercent" :show-value="false" class="stats-bar" />
      </div>
    </AppSurface>

    <AppSurface padding="lg">
      <div class="section-head">
        <div>
          <h2>结果分布</h2>
          <p>按当前可见提交统计，比赛进行中的他人赛时提交不计入。</p>
        </div>
      </div>

      <AppEmptyState
        v-if="!loading && totalVisible === 0"
        icon="pi pi-chart-pie"
        title="暂无可见提交"
        description="有提交后这里会显示各评测状态的占比。"
      />
      <div v-else-if="totalVisible > 0" class="charts">
        <div class="pie-wrap">
          <svg
            class="pie"
            viewBox="0 0 100 100"
            role="img"
            :aria-label="chartSummary"
            data-test="status-pie"
          >
            <circle
              cx="50"
              cy="50"
              r="40"
              fill="none"
              stroke="var(--kapoj-border)"
              stroke-width="20"
            />
            <circle
              v-for="slice in pieSlices"
              :key="slice.status"
              cx="50"
              cy="50"
              r="40"
              fill="none"
              :stroke="slice.color"
              stroke-width="20"
              :stroke-dasharray="`${slice.length} ${2 * Math.PI * 40 - slice.length}`"
              :stroke-dashoffset="-slice.offset"
              transform="rotate(-90 50 50)"
            />
          </svg>
          <div class="pie-center">
            <strong class="num">{{ totalVisible }}</strong>
            <span>可见提交</span>
          </div>
        </div>

        <div class="bars" data-test="status-bars">
          <div v-for="item in visibleCounts" :key="item.status" class="bar-row">
            <span class="bar-label">{{ statusLabel(item.status) }}</span>
            <div class="bar-track">
              <div
                class="bar-fill"
                :style="{
                  width: barWidth(item.count),
                  background: statusColor(item.status),
                }"
              />
            </div>
            <span class="bar-value num"
              >{{ item.count }} · {{ percentLabel(item.percentage) }}</span
            >
          </div>
        </div>
      </div>

      <ul v-if="totalVisible > 0" class="legend" data-test="status-legend">
        <li v-for="item in visibleCounts" :key="item.status">
          <span
            class="legend-dot"
            :style="{ background: statusColor(item.status) }"
            aria-hidden="true"
          />
          <span class="legend-label">{{ statusLabel(item.status) }}</span>
          <span class="legend-value num"
            >{{ item.count }} · {{ percentLabel(item.percentage) }}</span
          >
        </li>
      </ul>
    </AppSurface>

    <AppSurface padding="lg">
      <div class="section-head table-head">
        <div>
          <h2>提交明细</h2>
          <p>只展示耗时、内存、代码长度等指标，不提供代码入口。</p>
        </div>
        <div class="filters">
          <label class="filter">
            <span>排序</span>
            <select v-model="sort" aria-label="排序字段" data-test="sort-field">
              <option v-for="option in sortOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>
          <label class="filter">
            <span>方向</span>
            <select v-model="order" aria-label="排序方向" data-test="sort-order">
              <option value="desc">降序</option>
              <option value="asc">升序</option>
            </select>
          </label>
        </div>
      </div>

      <AppLoadingState v-if="metricsLoading && metrics.length === 0" label="正在加载提交明细" />
      <AppErrorState v-else-if="metricsError" :description="metricsError" @retry="loadMetrics" />
      <AppEmptyState
        v-else-if="metrics.length === 0"
        icon="pi pi-table"
        title="暂无提交明细"
        description="该题目暂时没有可见的提交记录。"
      />
      <template v-else>
        <DataTable
          :value="metrics"
          :loading="metricsLoading"
          size="small"
          row-hover
          responsive-layout="scroll"
          class="metrics-table"
          data-test="metrics-table"
        >
          <Column header="用户" style="min-width: 150px">
            <template #body="{ data }">
              <div class="user-cell">
                <Avatar
                  :image="resolvedAvatar(data.avatarUrl)"
                  :label="(data.nickname || data.username || '?').slice(0, 1)"
                  shape="circle"
                  class="user-avatar"
                />
                <UserIdentityName
                  :nickname="data.nickname"
                  :username="data.username"
                  :title="data.equippedTitle"
                  :badges="data.equippedBadges"
                  :rating-level="data.level"
                  size="small"
                  :link-to="{ name: 'user-profile', params: { id: data.userId } }"
                />
              </div>
            </template>
          </Column>
          <Column header="语言" style="width: 80px">
            <template #body="{ data }">{{ languageLabel(data.language) }}</template>
          </Column>
          <Column header="状态" style="width: 105px">
            <template #body="{ data }">
              <AppStatusTag :status="data.status" />
            </template>
          </Column>
          <Column header="耗时" style="width: 85px">
            <template #body="{ data }">
              <span class="num">{{
                data.language === 'answer' ? '—' : `${data.timeUsedMs} ms`
              }}</span>
            </template>
          </Column>
          <Column header="内存" style="width: 95px">
            <template #body="{ data }">
              <span class="num">{{
                data.language === 'answer' ? '—' : `${data.memoryUsedKb} KB`
              }}</span>
            </template>
          </Column>
          <Column header="代码长度" style="width: 100px">
            <template #body="{ data }">
              <span class="num">{{ codeLengthLabel(data.codeLength) }}</span>
            </template>
          </Column>
          <Column header="提交时间" style="width: 150px">
            <template #body="{ data }">
              <span class="num">{{ formatDateTime(data.createdAt) }}</span>
            </template>
          </Column>
        </DataTable>

        <Paginator
          v-if="metricTotal > 0"
          :rows="PAGE_SIZE"
          :total-records="metricTotal"
          :first="(page - 1) * PAGE_SIZE"
          :page-link-size="5"
          template="PrevPageLink PageLinks NextPageLink"
          class="pager"
          @page="onPage"
        />
      </template>
    </AppSurface>
  </div>
</template>

<style scoped>
.statistics-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
  margin: 0;
}

.stats-grid div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.stats-grid dt {
  color: var(--kapoj-text-muted);
  font-size: 10px;
  text-transform: uppercase;
}

.stats-grid dd {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.1rem;
  font-weight: 600;
}

.stats-bar {
  margin-top: var(--kapoj-space-4);
}

.section-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  margin-bottom: var(--kapoj-space-3);
}

.section-head h2 {
  margin: 0;
  font-size: 1rem;
}

.section-head p {
  margin: 4px 0 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.charts {
  display: grid;
  grid-template-columns: minmax(150px, 220px) minmax(0, 1fr);
  gap: var(--kapoj-space-5);
  align-items: center;
}

.pie-wrap {
  position: relative;
  width: 100%;
  max-width: 220px;
  aspect-ratio: 1;
  justify-self: center;
}

.pie {
  display: block;
  width: 100%;
  height: 100%;
}

.pie-center {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  pointer-events: none;
}

.pie-center strong {
  font-size: 1.4rem;
}

.pie-center span {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.bars {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.bar-row {
  display: grid;
  grid-template-columns: 74px minmax(0, 1fr) auto;
  gap: var(--kapoj-space-2);
  align-items: center;
  font-size: 12px;
}

.bar-label {
  overflow: hidden;
  color: var(--kapoj-text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bar-track {
  height: 8px;
  overflow: hidden;
  background: var(--kapoj-bg-elevated);
  border-radius: 999px;
}

.bar-fill {
  height: 100%;
  border-radius: inherit;
  transition: width var(--kapoj-transition);
}

.bar-value {
  color: var(--kapoj-text-muted);
  font-size: 11px;
  white-space: nowrap;
}

.legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2) var(--kapoj-space-4);
  padding: var(--kapoj-space-3) 0 0;
  margin: var(--kapoj-space-3) 0 0;
  list-style: none;
  border-top: 1px solid var(--kapoj-border);
}

.legend li {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}

.legend-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.legend-value {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.table-head {
  flex-wrap: wrap;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.filter {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.filter select {
  padding: 4px 8px;
  color: var(--kapoj-text);
  font: inherit;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}

.metrics-table {
  margin-top: var(--kapoj-space-2);
}

.user-cell {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.user-avatar {
  flex: none;
  width: 26px;
  height: 26px;
  font-size: 11px;
}

.pager {
  margin-top: var(--kapoj-space-3);
}

@media (max-width: 700px) {
  .stats-grid {
    gap: var(--kapoj-space-2);
  }

  .charts {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--kapoj-space-4);
  }

  .pie-wrap {
    max-width: 180px;
  }
}
</style>
