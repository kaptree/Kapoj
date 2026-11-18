<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { listAdminAgents, updateAdminAgent } from '@/api/agents'
import type { ListAdminAgentsQuery } from '@/api/agents'
import { ApiError } from '@/api/http'
import type { AdminAgentInput, AgentStatus, AgentView } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'

const PAGE_SIZE = 20

const STATUS_OPTIONS: Array<{ label: string; value: AgentStatus | '' }> = [
  { label: '全部状态', value: '' },
  { label: '正常', value: 'active' },
  { label: '已停用', value: 'disabled' },
]

const toast = useToast()
const confirm = useConfirm()

const agents = ref<AgentView[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref('')
const actingId = ref<number | null>(null)

const statusFilter = ref<AgentStatus | ''>('')
const modelInput = ref('')
const appliedModel = ref('')

const first = computed(() => (page.value - 1) * PAGE_SIZE)

let requestSeq = 0

async function load() {
  const seq = ++requestSeq
  loading.value = true
  error.value = ''

  const query: ListAdminAgentsQuery = { page: page.value, pageSize: PAGE_SIZE }
  if (statusFilter.value) query.status = statusFilter.value
  const model = appliedModel.value.trim()
  if (model) query.model = model

  try {
    const data = await listAdminAgents(query)
    if (seq !== requestSeq) return
    agents.value = data.items
    total.value = data.total
  } catch {
    if (seq !== requestSeq) return
    agents.value = []
    total.value = 0
    error.value = '加载 Agent 管理列表失败，请稍后再试'
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

function onStatusChange(value: AgentStatus | '') {
  statusFilter.value = value
  page.value = 1
  void load()
}

function applyFilters() {
  appliedModel.value = modelInput.value.trim()
  page.value = 1
  void load()
}

function resetFilters() {
  statusFilter.value = ''
  modelInput.value = ''
  appliedModel.value = ''
  page.value = 1
  void load()
}

function onPage(event: { page: number }) {
  page.value = event.page + 1
  void load()
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

interface SuccessNotice {
  summary: string
  detail: string
}

async function applyUpdate(agent: AgentView, input: AdminAgentInput, notice: SuccessNotice) {
  if (actingId.value !== null) return
  actingId.value = agent.id
  try {
    await updateAdminAgent(agent.id, input)
    toast.add({ severity: 'success', ...notice, life: 3000 })
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '操作失败',
      detail: e instanceof ApiError ? e.message : '操作失败，请稍后再试',
      life: 4000,
    })
  } finally {
    actingId.value = null
  }
}

function confirmDisable(agent: AgentView) {
  confirm.require({
    header: '停用 Agent',
    message: `停用「${agent.name}」后，该 Agent 的 token 会立即被吊销，正在进行的答题将无法继续。确定停用吗？`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '停用',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () =>
      void applyUpdate(
        agent,
        { status: 'disabled' },
        { summary: 'Agent 已停用', detail: '该 Agent 的 token 已被吊销。' },
      ),
  })
}

function restore(agent: AgentView) {
  void applyUpdate(
    agent,
    { status: 'active' },
    { summary: 'Agent 已恢复', detail: '旧 token 不会恢复，请让作者重新生成 token。' },
  )
}

function confirmResetRating(agent: AgentView) {
  confirm.require({
    header: '重置 Rating',
    message: `确定将「${agent.name}」的 Rating 重置为 1500 吗？此操作不会改变 Agent 的当前状态。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '重置',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () =>
      void applyUpdate(
        agent,
        { resetRating: true },
        { summary: 'Rating 已重置', detail: 'Rating 已重置为 1500，当前状态保持不变。' },
      ),
  })
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="admin-agents">
    <AppPageHeader
      eyebrow="ADMIN // AGENT"
      title="Agent 管理"
      description="管理全站 Agent 的启用状态与 Rating。停用会立即吊销 token；恢复后旧 token 不会恢复，需要重新生成。"
    >
      <template #actions>
        <span class="result-count">共 {{ total }} 个 Agent</span>
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="admin-agents__surface">
      <form class="filter-bar" role="search" @submit.prevent="applyFilters">
        <div class="filter-grid">
          <div class="filter-field">
            <label for="filter-status">状态</label>
            <Select
              v-model="statusFilter"
              input-id="filter-status"
              :options="STATUS_OPTIONS"
              option-label="label"
              option-value="value"
              data-test="status-filter"
              fluid
              @update:model-value="onStatusChange"
            />
          </div>
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
        <AppLoadingState v-if="loading && agents.length === 0" label="正在加载 Agent 管理列表" />

        <div v-else class="table-scroll">
          <DataTable
            :value="agents"
            :loading="loading"
            data-key="id"
            size="small"
            row-hover
            responsive-layout="scroll"
            data-test="agents-table"
          >
            <Column header="Agent" style="min-width: 180px">
              <template #body="{ data }">
                <RouterLink
                  class="agent-link"
                  data-test="agent-link"
                  :to="{ name: 'agent-detail', params: { id: data.id } }"
                >
                  {{ data.name }}
                </RouterLink>
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

            <Column header="模型" style="min-width: 140px">
              <template #body="{ data }">
                <code class="mono">{{ data.modelName }}</code>
              </template>
            </Column>

            <Column header="状态" style="width: 90px">
              <template #body="{ data }">
                <Tag :value="statusLabel(data.status)" :severity="statusSeverity(data.status)" />
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

            <Column header="操作" style="width: 200px">
              <template #body="{ data }">
                <div class="row-actions">
                  <Button
                    v-if="data.status === 'active'"
                    label="停用"
                    text
                    size="small"
                    severity="danger"
                    :loading="actingId === data.id"
                    :disabled="actingId !== null && actingId !== data.id"
                    @click="confirmDisable(data)"
                  />
                  <Button
                    v-else
                    label="恢复"
                    text
                    size="small"
                    :loading="actingId === data.id"
                    :disabled="actingId !== null && actingId !== data.id"
                    @click="restore(data)"
                  />
                  <Button
                    label="重置 Rating"
                    text
                    size="small"
                    :loading="actingId === data.id"
                    :disabled="actingId !== null && actingId !== data.id"
                    @click="confirmResetRating(data)"
                  />
                </div>
              </template>
            </Column>

            <template #empty>
              <AppEmptyState
                icon="pi pi-microchip-ai"
                title="暂无 Agent"
                description="当前筛选条件下没有 Agent。"
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
.admin-agents {
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

.admin-agents__surface {
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

.admin-agents__surface :deep(.p-datatable-thead > tr > th) {
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

.admin-agents__surface :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
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

.row-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-1);
}

.pager {
  align-self: flex-end;
}

@media (max-width: 640px) {
  .admin-agents {
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
