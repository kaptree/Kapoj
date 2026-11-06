<script setup lang="ts">
import { onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'

import { listAdminAudit } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AdminAuditItem } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatDateTime } from '@/utils/format'

const PAGE_SIZE = 20

const TARGET_TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: 'role', label: '角色' },
  { value: 'user', label: '用户' },
  { value: 'problem', label: '题目' },
  { value: 'contest', label: '比赛' },
  { value: 'gamification_achievement', label: '成就' },
  { value: 'gamification_task', label: '任务' },
  { value: 'gamification_badge', label: '徽章' },
  { value: 'gamification_title', label: '称号' },
  { value: 'learning_path', label: '学习路线' },
]

const filters = ref({
  actorId: null as number | null,
  action: '',
  targetType: null as string | null,
  from: null as Date | null,
  to: null as Date | null,
})

const items = ref<AdminAuditItem[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref('')

// Only the newest request may update the page; slow older ones are ignored.
let latest = 0

/** Format a Date using the local calendar day, e.g. `2026-02-05`. */
function toDateOnly(date: Date | null): string | undefined {
  if (!date) return undefined
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const actorId = filters.value.actorId
    const result = await listAdminAudit({
      actorId: actorId !== null && actorId > 0 ? actorId : undefined,
      action: filters.value.action.trim() || undefined,
      targetType: filters.value.targetType ?? undefined,
      from: toDateOnly(filters.value.from),
      to: toDateOnly(filters.value.to),
      page: page.value,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latest) return
    items.value = result.items
    total.value = result.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载审计日志失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function search() {
  page.value = 1
  void load()
}

function reset() {
  filters.value = { actorId: null, action: '', targetType: null, from: null, to: null }
  page.value = 1
  void load()
}

function onPage(event: { page: number }) {
  page.value = event.page + 1
  void load()
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="admin-audit">
    <AppPageHeader
      eyebrow="ADMIN // AUDIT"
      title="审计日志"
      description="查看平台管理操作的审计记录，按操作者、action、目标与日期筛选。"
    />

    <AppSurface padding="lg" class="admin-audit__surface">
      <form class="filter-bar" role="search" @submit.prevent="search">
        <div class="filter-fields">
          <InputNumber
            v-model="filters.actorId"
            input-id="audit-actor"
            class="actor-input"
            :min="1"
            :min-fraction-digits="0"
            :max-fraction-digits="0"
            placeholder="操作者 ID"
            aria-label="按操作者 ID 筛选"
            fluid
          />
          <InputText
            v-model="filters.action"
            class="action-input"
            fluid
            placeholder="action（权限码）"
            aria-label="按 action 筛选"
          />
          <Select
            v-model="filters.targetType"
            :options="TARGET_TYPE_OPTIONS"
            option-label="label"
            option-value="value"
            placeholder="目标类型"
            show-clear
            aria-label="按目标类型筛选"
            fluid
          />
          <DatePicker
            v-model="filters.from"
            input-id="audit-from"
            class="from-input"
            date-format="yy-mm-dd"
            placeholder="开始日期"
            aria-label="开始日期"
            fluid
          />
          <DatePicker
            v-model="filters.to"
            input-id="audit-to"
            class="to-input"
            date-format="yy-mm-dd"
            placeholder="结束日期"
            aria-label="结束日期"
            fluid
          />
        </div>
        <div class="filter-actions">
          <Button type="submit" label="搜索" icon="pi pi-search" />
          <Button type="button" label="重置" severity="secondary" text @click="reset" />
        </div>
      </form>

      <AppErrorState v-if="error" :description="error" @retry="load" />

      <AppLoadingState v-else-if="loading && items.length === 0" label="正在加载审计日志" />
      <DataTable
        v-else
        :value="items"
        :loading="loading"
        data-key="id"
        size="small"
        responsive-layout="scroll"
        class="admin-audit__table"
      >
        <Column header="时间" style="width: 170px">
          <template #body="{ data }">
            <span class="muted">{{ formatDateTime(data.createdAt) }}</span>
          </template>
        </Column>
        <Column header="操作者" style="width: 110px">
          <template #body="{ data }">
            <span class="num">#{{ data.actorId }}</span>
          </template>
        </Column>
        <Column header="action" style="min-width: 150px">
          <template #body="{ data }">
            <code class="action">{{ data.action }}</code>
          </template>
        </Column>
        <Column header="目标" style="min-width: 150px">
          <template #body="{ data }">
            <span>{{ data.targetType }}</span>
            <span v-if="data.targetId != null" class="muted"> · #{{ data.targetId }}</span>
          </template>
        </Column>
        <Column header="摘要" style="min-width: 180px">
          <template #body="{ data }">
            <span class="summary">{{ data.summary }}</span>
          </template>
        </Column>
        <Column header="IP / UA" style="min-width: 220px">
          <template #body="{ data }">
            <div class="ip-ua">
              <span class="ip">{{ data.ip }}</span>
              <span class="ua" :title="data.userAgent">{{ data.userAgent }}</span>
            </div>
          </template>
        </Column>
        <template #empty>
          <AppEmptyState
            icon="pi pi-clock"
            title="暂无审计记录"
            description="调整筛选条件后重试，或稍后再来查看。"
          />
        </template>
      </DataTable>

      <Paginator
        v-if="!error && total > PAGE_SIZE"
        :rows="PAGE_SIZE"
        :total-records="total"
        :first="(page - 1) * PAGE_SIZE"
        :page-link-size="5"
        template="PrevPageLink PageLinks NextPageLink"
        class="admin-audit__pager"
        @page="onPage"
      />
    </AppSurface>
  </div>
</template>

<style scoped>
.admin-audit {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.admin-audit__surface {
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
}

.filter-fields {
  display: flex;
  flex: 1 1 420px;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.filter-fields > * {
  flex: 1 1 180px;
  min-width: 0;
}

.filter-actions {
  display: flex;
  gap: var(--kapoj-space-2);
}

.admin-audit__table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.admin-audit__table :deep(.p-datatable-thead > tr > th) {
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

.admin-audit__table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.ip-ua {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.ip {
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.ua {
  overflow: hidden;
  color: var(--kapoj-text-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.action {
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.summary {
  color: var(--kapoj-text);
  font-size: 13px;
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.admin-audit__pager {
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}
</style>
