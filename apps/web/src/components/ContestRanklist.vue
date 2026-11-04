<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { getContestRanklist } from '@/api/contests'
import { ApiError } from '@/api/http'
import type { ContestRankCell, ContestRanklist, ContestRankRow } from '@/api/types'
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

function cellText(cell: ContestRankCell, icpc: boolean): string {
  if (icpc) {
    if (cell.status === 'accepted') return `AC (${cell.attempts})`
    return cell.attempts > 0 ? `-${cell.attempts}` : ''
  }
  return String(cell.score || '')
}

function rowValue(row: ContestRankRow): string {
  if (!ranklist.value) return ''
  if (ranklist.value.mode === 'icpc') return `${row.solved} / ${row.penalty}`
  return String(row.totalScore)
}

onBeforeUnmount(() => {
  latest++
})
</script>

<template>
  <div v-loading="loading">
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />
    <template v-if="ranklist">
      <div class="meta">
        <span>{{ ranklist.mode.toUpperCase() }}</span>
        <span>生成于 {{ formatDateTime(ranklist.generatedAt) }}</span>
        <el-tag v-if="ranklist.selfOnly" type="warning" size="small" effect="plain">
          OI 赛时仅显示本人成绩，赛后公布完整榜单
        </el-tag>
      </div>
      <el-table :data="ranklist.rows" row-key="userId" empty-text="暂无参赛者">
        <el-table-column prop="rank" label="#" width="60" />
        <el-table-column label="选手" min-width="140">
          <template #default="{ row }">{{ row.nickname || row.username }}</template>
        </el-table-column>
        <el-table-column :label="ranklist.mode === 'icpc' ? '通过 / 罚时' : '总分'" width="120">
          <template #default="{ row }">{{ rowValue(row) }}</template>
        </el-table-column>
        <el-table-column
          v-for="(problem, index) in ranklist.problems"
          :key="problem.problemId"
          :label="problem.displayId"
          width="92"
          align="center"
        >
          <template #default="{ row }">
            <span :class="{ accepted: row.problems[index]?.status === 'accepted' }">
              {{ cellText(row.problems[index], ranklist.mode === 'icpc') }}
            </span>
          </template>
        </el-table-column>
      </el-table>
    </template>
  </div>
</template>

<style scoped>
.banner {
  margin-bottom: 12px;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  margin-bottom: 8px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.accepted {
  color: var(--el-color-success);
  font-weight: 600;
}
</style>
