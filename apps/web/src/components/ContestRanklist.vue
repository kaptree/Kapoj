<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Tag from 'primevue/tag'

import { getContestRanklist } from '@/api/contests'
import { ApiError } from '@/api/http'
import type { ContestRankCell, ContestRankRow, ContestRanklist } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import IdentityShowcase from '@/components/IdentityShowcase.vue'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{ contestId: number; refreshTick: number }>()

const ranklist = ref<ContestRanklist | null>(null)
const loading = ref(false)
const error = ref('')
let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await getContestRanklist(props.contestId)
    if (seq === latest) ranklist.value = data
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载榜单失败'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  () => props.refreshTick,
  () => void load(),
)
onMounted(load)

function cellText(cell: ContestRankCell | undefined, icpc: boolean): string {
  if (!cell) return ''
  if (icpc) {
    if (cell.status === 'accepted') return `-${cell.attempts}`
    return cell.attempts > 0 ? `-${cell.attempts}` : ''
  }
  return cell.attempts > 0 ? String(cell.score) : ''
}

function cellClass(cell: ContestRankCell | undefined) {
  if (!cell) return {}
  return {
    'cell--first': cell.status === 'accepted' && cell.firstBlood,
    'cell--accepted': cell.status === 'accepted' && !cell.firstBlood,
    'cell--failed': cell.status !== 'accepted' && cell.attempts > 0,
  }
}

function rowValue(row: ContestRankRow): string {
  if (!ranklist.value) return ''
  if (ranklist.value.mode === 'icpc') return `${row.solved} / ${row.penalty}`
  return String(row.totalScore)
}

function acceptedAt(row: ContestRankRow, index: number): string {
  const cell = row.problems[index]
  return cell?.acceptedAt ? formatDateTime(cell.acceptedAt) : ''
}

onBeforeUnmount(() => {
  latest++
})
</script>

<template>
  <div class="contest-ranklist">
    <AppErrorState v-if="error && !ranklist" :description="error" @retry="load" />

    <AppLoadingState v-else-if="loading && !ranklist" label="正在加载榜单" />

    <template v-else-if="ranklist">
      <Message v-if="error" severity="error" :closable="false">
        {{ error }}
      </Message>
      <div class="ranklist-meta">
        <span class="ranklist-mode">{{ ranklist.mode.toUpperCase() }}</span>
        <span>生成于 {{ formatDateTime(ranklist.generatedAt) }}</span>
        <span class="ranklist-legend">
          <span class="legend-item"><i class="legend-swatch legend-swatch--first" />首杀</span>
          <span class="legend-item"><i class="legend-swatch legend-swatch--accepted" />通过</span>
          <span class="legend-item"><i class="legend-swatch legend-swatch--failed" />未通过</span>
        </span>
        <Tag
          v-if="ranklist.selfOnly"
          value="OI 赛时仅显示本人成绩，赛后公布完整榜单"
          severity="warn"
        />
      </div>

      <DataTable
        :value="ranklist.rows"
        :loading="loading"
        data-key="userId"
        size="small"
        responsive-layout="scroll"
        class="ranklist-table"
      >
        <Column header="#" style="width: 60px">
          <template #body="{ data }">
            <span class="num">{{ data.rank }}</span>
          </template>
        </Column>
        <Column header="选手" style="min-width: 160px">
          <template #body="{ data }">
            <div class="player-cell">
              <span class="player-name">{{ data.nickname || data.username }}</span>
              <IdentityShowcase
                :badges="data.equippedBadges"
                :title="data.equippedTitle"
                size="small"
              />
            </div>
          </template>
        </Column>
        <Column :header="ranklist.mode === 'icpc' ? '通过 / 罚时' : '总分'" style="width: 120px">
          <template #body="{ data }">
            <span class="num">{{ rowValue(data) }}</span>
          </template>
        </Column>
        <Column
          v-for="(problem, index) in ranklist.problems"
          :key="problem.problemId"
          :header="problem.displayId"
          style="width: 92px"
        >
          <template #body="{ data }">
            <span
              class="cell"
              :class="cellClass(data.problems[index])"
              :title="acceptedAt(data, index)"
            >
              {{ cellText(data.problems[index], ranklist.mode === 'icpc') }}
            </span>
          </template>
        </Column>
        <template #empty>
          <AppEmptyState
            icon="pi pi-users"
            title="暂无参赛者"
            description="比赛开始后，参赛者与成绩会显示在这里。"
          />
        </template>
      </DataTable>
    </template>
  </div>
</template>

<style scoped>
.contest-ranklist {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.ranklist-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.ranklist-mode {
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-weight: 600;
  letter-spacing: 0.08em;
}

.ranklist-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.ranklist-table :deep(.p-datatable-thead > tr > th) {
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

.ranklist-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.player-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  min-width: 0;
}

.player-name {
  color: var(--kapoj-text);
  font-weight: 600;
}

.cell {
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.cell--first,
.cell--accepted,
.cell--failed {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 4px;
}

.cell--first {
  color: var(--kapoj-success);
  font-weight: 700;
  background: color-mix(in srgb, var(--kapoj-success) 20%, transparent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--kapoj-success) 40%, transparent);
}

.cell--accepted {
  color: var(--kapoj-success);
  background: color-mix(in srgb, var(--kapoj-success) 10%, transparent);
}

.cell--failed {
  color: var(--kapoj-danger);
  background: color-mix(in srgb, var(--kapoj-danger) 10%, transparent);
}

.ranklist-legend {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.legend-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.legend-swatch {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 3px;
}

.legend-swatch--first {
  background: color-mix(in srgb, var(--kapoj-success) 55%, transparent);
  box-shadow: inset 0 0 0 1px var(--kapoj-success);
}

.legend-swatch--accepted {
  background: color-mix(in srgb, var(--kapoj-success) 20%, transparent);
}

.legend-swatch--failed {
  background: color-mix(in srgb, var(--kapoj-danger) 20%, transparent);
}

@media (max-width: 640px) {
  .ranklist-meta {
    gap: var(--kapoj-space-2);
  }
}
</style>
