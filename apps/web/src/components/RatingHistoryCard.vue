<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'

import { ApiError } from '@/api/http'
import { getRatingHistory } from '@/api/leaderboard'
import type { RatingHistoryItem } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatDateTime } from '@/utils/format'
import { formatDelta } from '@/utils/rating'

const PAGE_SIZE = 20
const CHART_WIDTH = 320
const CHART_HEIGHT = 80
const CHART_PADDING = 10

const props = defineProps<{
  userId: number
  rating?: number
}>()

interface ChartPoint {
  x: number
  y: number
}

interface ChartData {
  points: ChartPoint[]
  polyline: string
  label: string
}

const items = ref<RatingHistoryItem[]>([])
const loading = ref(false)
const loaded = ref(false)
const error = ref('')
let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const page = await getRatingHistory(props.userId, { page: 1, pageSize: PAGE_SIZE })
    if (seq !== latest) return
    items.value = page.items
    loaded.value = true
  } catch (e) {
    if (seq !== latest) return
    items.value = []
    loaded.value = false
    error.value = e instanceof ApiError ? e.message : '加载 Rating 历史失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  () => props.userId,
  () => {
    items.value = []
    loaded.value = false
    void load()
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  latest += 1
})

// The API returns newest first; the chart reads left-to-right chronologically.
const chronological = computed(() => [...items.value].reverse())

const chartData = computed<ChartData | null>(() => {
  const entries = chronological.value
  if (entries.length < 2) return null

  const ratings = entries.map((entry) => entry.ratingAfter)
  const min = Math.min(...ratings)
  const max = Math.max(...ratings)
  const span = max - min
  const innerWidth = CHART_WIDTH - CHART_PADDING * 2
  const innerHeight = CHART_HEIGHT - CHART_PADDING * 2

  const points: ChartPoint[] = entries.map((entry, index) => ({
    x: CHART_PADDING + (index / (entries.length - 1)) * innerWidth,
    y:
      span === 0
        ? CHART_HEIGHT / 2
        : CHART_PADDING + (1 - (entry.ratingAfter - min) / span) * innerHeight,
  }))

  const first = entries[0].ratingAfter
  const last = entries[entries.length - 1].ratingAfter
  return {
    points,
    polyline: points.map((point) => `${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(' '),
    label: `Rating 历史曲线：共 ${entries.length} 场比赛，Rating 从 ${first} 到 ${last}。`,
  }
})

function deltaClass(delta: number): string {
  if (delta > 0) return 'delta--positive'
  if (delta < 0) return 'delta--negative'
  return 'delta--neutral'
}

function contestName(item: RatingHistoryItem): string {
  return item.contestId === 0 ? '私密比赛' : item.contestTitle || '未命名比赛'
}
</script>

<template>
  <AppSurface padding="lg" class="rating-history">
    <header class="rating-history__head">
      <div class="rating-history__heading">
        <h2 class="rating-history__title">Rating 历史</h2>
        <p class="rating-history__hint">最近 {{ PAGE_SIZE }} 场 Rated 比赛的 Rating 变化</p>
      </div>
      <p v-if="rating !== undefined" class="rating-history__current">
        当前 Rating
        <strong class="num">{{ rating }}</strong>
      </p>
    </header>

    <AppLoadingState v-if="loading && !loaded" label="正在加载 Rating 历史" />

    <AppErrorState v-else-if="error" :description="error" @retry="load" />

    <AppEmptyState
      v-else-if="items.length === 0"
      icon="pi pi-chart-line"
      title="暂无 Rating 记录"
      description="参加 Rated 比赛并完成提交后，这里会显示 Rating 变化。"
    />

    <template v-else>
      <figure v-if="chartData" class="rating-history__chart">
        <svg
          class="rating-chart"
          :viewBox="`0 0 ${CHART_WIDTH} ${CHART_HEIGHT}`"
          role="img"
          :aria-label="chartData.label"
        >
          <title>{{ chartData.label }}</title>
          <polyline class="rating-chart__line" :points="chartData.polyline" fill="none" />
          <circle
            v-for="(point, index) in chartData.points"
            :key="index"
            class="rating-chart__dot"
            :cx="point.x"
            :cy="point.y"
            r="2.5"
          />
        </svg>
      </figure>

      <div
        class="rating-table-scroll"
        tabindex="0"
        role="region"
        aria-label="Rating 历史表格，可横向滚动"
      >
        <DataTable :value="items" size="small" class="rating-history-table">
          <Column header="比赛" style="min-width: 180px">
            <template #body="{ data }">
              <RouterLink
                v-if="data.contestId > 0"
                :to="{ name: 'contest-detail', params: { id: data.contestId } }"
                class="contest-link"
              >
                {{ contestName(data) }}
              </RouterLink>
              <span v-else class="contest-private">{{ contestName(data) }}</span>
            </template>
          </Column>

          <Column header="名次" style="width: 72px">
            <template #body="{ data }">
              <span class="num">{{ data.rank }}</span>
            </template>
          </Column>

          <Column header="赛前→赛后" style="width: 140px">
            <template #body="{ data }">
              <span class="rating-pair">
                {{ data.ratingBefore }}
                <span class="rating-pair__arrow" aria-hidden="true">→</span>
                {{ data.ratingAfter }}
              </span>
            </template>
          </Column>

          <Column header="变化" style="width: 88px">
            <template #body="{ data }">
              <span class="delta" :class="deltaClass(data.delta)">
                {{ formatDelta(data.delta) }}
              </span>
            </template>
          </Column>

          <Column header="时间" style="min-width: 150px">
            <template #body="{ data }">
              <span class="time">{{ formatDateTime(data.createdAt) }}</span>
            </template>
          </Column>
        </DataTable>
      </div>
    </template>
  </AppSurface>
</template>

<style scoped>
.rating-history {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.rating-history__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--kapoj-space-2) var(--kapoj-space-4);
}

.rating-history__heading {
  min-width: 0;
}

.rating-history__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.05rem;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.rating-history__hint {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-muted);
  font-size: 0.8125rem;
}

.rating-history__current {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.rating-history__current strong {
  color: var(--kapoj-text);
  font-size: 1rem;
}

.rating-history__chart {
  margin: 0;
  padding: var(--kapoj-space-2) var(--kapoj-space-3);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.rating-chart {
  display: block;
  width: 100%;
  height: auto;
  overflow: visible;
}

.rating-chart__line {
  stroke: var(--kapoj-primary);
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.rating-chart__dot {
  fill: var(--kapoj-primary);
}

.rating-table-scroll {
  overflow-x: auto;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.rating-table-scroll:focus-visible {
  outline: 2px solid var(--kapoj-primary);
  outline-offset: 2px;
}

/* Let the wrapper own horizontal scrolling on narrow screens. */
.rating-table-scroll :deep(.p-datatable-table-container) {
  overflow: visible !important;
}

.rating-history-table :deep(.p-datatable-table) {
  min-width: 560px;
}

.rating-history-table :deep(.p-datatable-thead > tr > th) {
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

.rating-history-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  border-color: var(--kapoj-border);
}

.contest-link {
  color: var(--kapoj-text);
  font-weight: 600;
  word-break: break-word;
}

.contest-link:hover {
  color: var(--kapoj-primary);
}

.contest-private {
  color: var(--kapoj-text-muted);
}

.rating-pair {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  white-space: nowrap;
}

.rating-pair__arrow {
  margin: 0 2px;
  color: var(--kapoj-text-muted);
}

.delta {
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  font-weight: 600;
}

.delta--positive {
  color: var(--kapoj-success);
}

.delta--negative {
  color: var(--kapoj-danger);
}

.delta--neutral {
  color: var(--kapoj-text-muted);
}

.time {
  color: var(--kapoj-text-muted);
  font-size: 0.75rem;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .rating-history {
    gap: var(--kapoj-space-3);
  }

  .rating-history__chart {
    padding: var(--kapoj-space-2);
  }
}
</style>
