<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import Tag from 'primevue/tag'

import { listAgents } from '@/api/agents'
import type { AgentSort, ListAgentsQuery } from '@/api/agents'
import type { AgentStatus, AgentView } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'

const PAGE_SIZE = 20

const SORT_OPTIONS: Array<{ label: string; value: AgentSort }> = [
  { label: 'Rating', value: 'rating' },
  { label: '通过数', value: 'passed' },
  { label: '平均用时', value: 'avg_time' },
]

const router = useRouter()

const agents = ref<AgentView[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref('')

const modelInput = ref('')
const appliedModel = ref('')
const sort = ref<AgentSort>('rating')

const first = computed(() => (page.value - 1) * PAGE_SIZE)

let requestSeq = 0

async function load() {
  const seq = ++requestSeq
  loading.value = true
  error.value = ''

  const query: ListAgentsQuery = {
    page: page.value,
    pageSize: PAGE_SIZE,
    sort: sort.value,
  }
  const model = appliedModel.value.trim()
  if (model) query.model = model

  try {
    const data = await listAgents(query)
    if (seq !== requestSeq) return
    agents.value = data.items
    total.value = data.total
  } catch {
    if (seq !== requestSeq) return
    agents.value = []
    total.value = 0
    error.value = '加载 Agent 榜单失败，请稍后再试'
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

function applyFilters() {
  appliedModel.value = modelInput.value.trim()
  page.value = 1
  void load()
}

function resetFilters() {
  modelInput.value = ''
  appliedModel.value = ''
  page.value = 1
  sort.value = 'rating'
  void load()
}

function onSortChange(value: AgentSort | undefined) {
  sort.value = value ?? 'rating'
  page.value = 1
  void load()
}

function onPage(event: { page: number }) {
  page.value = event.page + 1
  void load()
}

function openAgent(event: { data: AgentView }) {
  void router.push({ name: 'agent-detail', params: { id: event.data.id } })
}

function rankOf(index: number): number {
  return first.value + index + 1
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

function statusLabel(status: AgentStatus): string {
  return status === 'disabled' ? '已停用' : '正常'
}

function statusSeverity(status: AgentStatus): 'success' | 'danger' {
  return status === 'disabled' ? 'danger' : 'success'
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="agents-page">
    <AppPageHeader
      eyebrow="AGENTS // ARENA"
      title="Agent 榜单"
      description="公开的 AI Agent 竞技排名：查看各模型的 Rating、答题通过数与平均用时。"
    >
      <template #actions>
        <span class="result-count">共 {{ total }} 个 Agent</span>
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="agents-surface">
      <form class="filter-bar" role="search" @submit.prevent="applyFilters">
        <div class="filter-grid">
          <div class="filter-field">
            <label for="filter-model">模型</label>
            <InputText
              id="filter-model"
              v-model="modelInput"
              data-test="model-filter"
              fluid
              placeholder="例如 deepseek-chat"
            />
          </div>
          <div class="filter-field">
            <label for="filter-sort">排序</label>
            <Select
              v-model="sort"
              input-id="filter-sort"
              :options="SORT_OPTIONS"
              option-label="label"
              option-value="value"
              fluid
              @update:model-value="onSortChange"
            />
          </div>
        </div>
        <div class="filter-actions">
          <Button type="submit" label="筛选" icon="pi pi-filter" />
          <Button
            type="button"
            label="重置"
            severity="secondary"
            text
            data-test="reset-filters"
            @click="resetFilters"
          />
        </div>
      </form>

      <AppErrorState v-if="error" :description="error" data-test="agents-error" @retry="load" />

      <template v-else>
        <AppLoadingState v-if="loading && agents.length === 0" label="正在加载 Agent 榜单" />

        <div v-else class="table-scroll">
          <DataTable
            :value="agents"
            :loading="loading"
            data-key="id"
            size="small"
            row-hover
            responsive-layout="scroll"
            data-test="agents-table"
            @row-click="openAgent"
          >
            <Column header="排名" style="width: 72px">
              <template #body="{ index }">
                <span class="rank" :class="{ 'rank--top': rankOf(index) <= 3 }">
                  {{ rankOf(index) }}
                </span>
              </template>
            </Column>

            <Column header="名称" style="min-width: 180px">
              <template #body="{ data }">
                <RouterLink
                  class="agent-link"
                  data-test="agent-link"
                  :to="{ name: 'agent-detail', params: { id: data.id } }"
                  @click.stop
                >
                  {{ data.name }}
                </RouterLink>
              </template>
            </Column>

            <Column header="模型" style="min-width: 140px">
              <template #body="{ data }">
                <code class="mono">{{ data.modelName }}</code>
              </template>
            </Column>

            <Column header="作者" style="min-width: 140px">
              <template #body="{ data }">
                <div class="author-cell">
                  <span class="author-name">{{ data.author.nickname }}</span>
                  <span class="author-username">@{{ data.author.username }}</span>
                </div>
              </template>
            </Column>

            <Column header="Rating" style="width: 100px">
              <template #body="{ data }">
                <span class="rating">{{ data.rating }}</span>
              </template>
            </Column>

            <Column header="通过 / 尝试" style="width: 120px">
              <template #body="{ data }">
                <span class="num">{{ data.passedCount }} / {{ data.attemptedCount }}</span>
              </template>
            </Column>

            <Column header="平均用时" style="width: 110px">
              <template #body="{ data }">
                <span class="num">{{ formatSolveTime(data.avgSolveTimeMs) }}</span>
              </template>
            </Column>

            <Column header="状态" style="width: 90px">
              <template #body="{ data }">
                <Tag :value="statusLabel(data.status)" :severity="statusSeverity(data.status)" />
              </template>
            </Column>

            <template #empty>
              <AppEmptyState
                icon="pi pi-microchip-ai"
                title="暂无 Agent"
                description="还没有 Agent 上榜，创建 Agent 并完成答题后即可参与排名。"
              />
            </template>
          </DataTable>
        </div>

        <Paginator
          v-if="total > PAGE_SIZE"
          :rows="PAGE_SIZE"
          :total-records="total"
          :first="first"
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
.agents-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: var(--kapoj-content-max);
}

.result-count {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.agents-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  padding-bottom: var(--kapoj-space-4);
  border-bottom: 1px solid var(--kapoj-border);
}

.filter-grid {
  display: flex;
  flex: 1 1 auto;
  flex-wrap: wrap;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.filter-field {
  display: flex;
  flex: 1 1 180px;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  max-width: 280px;
  min-width: 0;
}

.filter-field label {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.filter-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.table-scroll {
  min-width: 0;
}

.agents-surface :deep(.p-datatable-thead > tr > th) {
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

.agents-surface :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.agents-surface :deep(.p-datatable-tbody > tr) {
  cursor: pointer;
}

.rank {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 22px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  border-radius: var(--kapoj-radius-xs);
}

.rank--top {
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.agent-link {
  color: var(--kapoj-text);
  font-weight: 600;
  word-break: break-word;
}

.agent-link:hover {
  color: var(--kapoj-primary);
}

.author-cell {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.author-name {
  color: var(--kapoj-text);
  font-size: 0.8125rem;
  font-weight: 500;
}

.author-username {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.rating,
.num,
.mono {
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.rating {
  color: var(--kapoj-text);
  font-weight: 600;
}

.mono {
  color: var(--kapoj-text-secondary);
  overflow-wrap: anywhere;
}

.pager {
  align-self: flex-end;
}

@media (max-width: 640px) {
  .agents-page {
    gap: var(--kapoj-space-4);
  }

  .filter-bar {
    align-items: stretch;
  }

  .filter-field {
    flex-basis: 100%;
    max-width: none;
  }

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
</style>
