<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { cloneContest, deleteContest, listContests } from '@/api/contests'
import { ApiError } from '@/api/http'
import type { ContestListItem, ContestMode, ContestStatus } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import {
  contestModeLabel,
  contestStatusLabel,
  CONTEST_MODES,
  CONTEST_STATUSES,
  registerTypeLabel,
} from '@/utils/contest'
import { formatDateTime } from '@/utils/format'

const PAGE_SIZE = 20
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()

const canViewHidden = computed(() => auth.can(PERM.contest.viewHidden))
const canCreate = computed(() => auth.can(PERM.contest.create))
const canEdit = computed(() => auth.can(PERM.contest.update))
const canDelete = computed(() => auth.can(PERM.contest.delete))
const canAnyAction = computed(() => canCreate.value || canEdit.value || canDelete.value)
const isLoggedIn = computed(() => auth.isLoggedIn)

interface Filters {
  keyword: string
  mode: ContestMode | '' | null
  status: ContestStatus | '' | null
  mine: boolean
  includeHidden: boolean
}

function emptyFilters(): Filters {
  return { keyword: '', mode: '', status: '', mine: false, includeHidden: false }
}

const filters = ref<Filters>(emptyFilters())
const applied = ref<Filters & { page: number }>({ ...emptyFilters(), page: 1 })
const first = computed(() => (applied.value.page - 1) * PAGE_SIZE)
const items = ref<ContestListItem[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')

const cloneDialog = ref(false)
const cloneTitle = ref('')
const cloneError = ref('')
const cloneTarget = ref<ContestListItem | null>(null)
const cloning = ref(false)

let latest = 0

function firstValue(value: unknown): string {
  const raw = Array.isArray(value) ? value[0] : value
  return typeof raw === 'string' ? raw : ''
}

function boolQuery(value: unknown): boolean {
  return firstValue(value) === 'true'
}

function intQuery(value: unknown, fallback: number): number {
  const n = Number(firstValue(value))
  return Number.isInteger(n) && n >= 1 ? n : fallback
}

function applyQuery() {
  const parsed: Filters = {
    keyword: firstValue(route.query.keyword),
    mode: (firstValue(route.query.mode) as ContestMode | '') || '',
    status: (firstValue(route.query.status) as ContestStatus | '') || '',
    mine: boolQuery(route.query.mine),
    includeHidden: boolQuery(route.query.includeHidden),
  }
  Object.assign(filters.value, parsed)
  Object.assign(applied.value, parsed)
  applied.value.page = intQuery(route.query.page, 1)
}

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listContests({
      keyword: applied.value.keyword,
      mode: applied.value.mode || '',
      status: applied.value.status || '',
      mine: applied.value.mine,
      // Defense-in-depth: never request hidden contests without the permission,
      // even if the query string asks for it.
      includeHidden: canViewHidden.value && applied.value.includeHidden,
      page: applied.value.page,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latest) return
    items.value = data.items
    total.value = data.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载比赛失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function queryFrom(state: Filters, page: number): Record<string, string> {
  const q: Record<string, string> = {}
  if (state.keyword.trim()) q.keyword = state.keyword.trim()
  if (state.mode) q.mode = state.mode
  if (state.status) q.status = state.status
  if (state.mine) q.mine = 'true'
  if (state.includeHidden) q.includeHidden = 'true'
  if (page > 1) q.page = String(page)
  return q
}

function search() {
  void router.push({ name: 'contests', query: queryFrom(filters.value, 1) })
}

function reset() {
  const wasEmpty = Object.keys(route.query).length === 0
  Object.assign(filters.value, emptyFilters())
  Object.assign(applied.value, emptyFilters())
  applied.value.page = 1
  if (wasEmpty) {
    void load()
    return
  }
  void router.push({ name: 'contests', query: {} })
}

function changePage(page: number) {
  void router.push({ name: 'contests', query: queryFrom(applied.value, page) })
}

function onPage(event: { page: number }) {
  changePage(event.page + 1)
}

function openDetail(row: ContestListItem) {
  void router.push({ name: 'contest-detail', params: { id: row.id } })
}

function onRowClick(event: { data: ContestListItem }) {
  openDetail(event.data)
}

function edit(row: ContestListItem) {
  void router.push({ name: 'contest-edit', params: { id: row.id } })
}

function createContest() {
  void router.push({ name: 'contest-create' })
}

function openClone(row: ContestListItem) {
  cloneTarget.value = row
  cloneTitle.value = `${row.title}（副本）`
  cloneError.value = ''
  cloneDialog.value = true
}

function resetClone() {
  cloneTarget.value = null
  cloneTitle.value = ''
  cloneError.value = ''
}

async function submitClone() {
  const target = cloneTarget.value
  if (!target) return
  const title = cloneTitle.value.trim()
  if (!title) {
    cloneError.value = '请输入比赛标题'
    return
  }

  cloning.value = true
  cloneError.value = ''
  try {
    const created = await cloneContest(target.id, { title })
    toast.add({ severity: 'success', summary: '比赛已克隆', life: 3000 })
    cloneDialog.value = false
    await router.push({ name: 'contest-edit', params: { id: created.id } })
  } catch (e) {
    cloneError.value = e instanceof ApiError ? e.message : '克隆失败'
    toast.add({
      severity: 'error',
      summary: '克隆失败',
      detail: cloneError.value,
      life: 4000,
    })
  } finally {
    cloning.value = false
  }
}

function remove(row: ContestListItem) {
  confirm.require({
    header: '删除比赛',
    message: `确定删除比赛「${row.title}」吗？`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void doRemove(row)
    },
  })
}

async function doRemove(row: ContestListItem) {
  try {
    await deleteContest(row.id)
    toast.add({ severity: 'success', summary: '已删除', life: 3000 })
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '删除失败',
      detail: e instanceof ApiError ? e.message : '删除失败',
      life: 4000,
    })
  }
}

function contestTagSeverity(status: ContestStatus): 'success' | 'warn' | 'info' {
  if (status === 'running') return 'success'
  if (status === 'upcoming') return 'warn'
  return 'info'
}

watch(
  () => route.query,
  () => {
    applyQuery()
    void load()
  },
  { immediate: true },
)
</script>

<template>
  <div class="contests-page">
    <AppPageHeader
      eyebrow="CONTESTS // ARENA"
      title="比赛"
      description="浏览公开比赛、筛选赛制与状态，或管理你创建的比赛。"
    >
      <template #actions>
        <span v-if="isLoggedIn" class="notice-chip">我报名的比赛可在此筛选</span>
        <span v-if="!loading || total > 0" class="result-count">共 {{ total }} 场比赛</span>
        <Button v-if="canCreate" label="新建比赛" icon="pi pi-plus" @click="createContest" />
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="contests-surface">
      <form class="filter-bar" role="search" @submit.prevent="search">
        <div class="filter-grid">
          <div class="filter-field">
            <label for="filter-keyword">关键词</label>
            <InputText id="filter-keyword" v-model="filters.keyword" fluid placeholder="比赛标题" />
          </div>
          <div class="filter-field">
            <label for="filter-mode">赛制</label>
            <Select
              v-model="filters.mode"
              input-id="filter-mode"
              :options="CONTEST_MODES"
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
              :options="CONTEST_STATUSES"
              option-label="label"
              option-value="value"
              show-clear
              fluid
            />
          </div>
          <div v-if="isLoggedIn" class="filter-check">
            <Checkbox v-model="filters.mine" input-id="filter-mine" binary />
            <label for="filter-mine">只看我报名的</label>
          </div>
          <div v-if="canViewHidden" class="filter-check">
            <Checkbox v-model="filters.includeHidden" input-id="filter-hidden" binary />
            <label for="filter-hidden">包含非公开</label>
          </div>
        </div>
        <div class="filter-actions">
          <Button type="submit" label="筛选" icon="pi pi-filter" />
          <Button type="button" label="重置" severity="secondary" text @click="reset" />
        </div>
      </form>

      <AppErrorState v-if="error" :description="error" @retry="load" />

      <AppLoadingState v-if="loading && items.length === 0" label="正在加载比赛" />
      <DataTable
        v-else-if="!error || items.length > 0"
        :value="items"
        :loading="loading"
        data-key="id"
        size="small"
        row-hover
        responsive-layout="scroll"
        class="contests-table"
        @row-click="onRowClick"
      >
        <Column header="比赛" style="min-width: 220px">
          <template #body="{ data }">
            <div class="title-line">
              <span class="contest-title">{{ data.title }}</span>
              <Tag v-if="data.registered" value="已报名" severity="success" />
            </div>
            <div class="sub">{{ registerTypeLabel(data.registerType) }}</div>
          </template>
        </Column>
        <Column header="赛制" style="width: 70px">
          <template #body="{ data }">{{ contestModeLabel(data.mode) }}</template>
        </Column>
        <Column header="状态" style="width: 80px">
          <template #body="{ data }">
            <Tag
              :value="contestStatusLabel(data.status)"
              :severity="contestTagSeverity(data.status)"
            />
          </template>
        </Column>
        <Column header="比赛时间" style="min-width: 190px">
          <template #body="{ data }">
            <div class="time">{{ formatDateTime(data.startTime) }}</div>
            <div class="time">至 {{ formatDateTime(data.endTime) }}</div>
          </template>
        </Column>
        <Column header="规模" style="width: 100px">
          <template #body="{ data }">
            <span class="num">{{ data.problemCount }} 题 / {{ data.participantCount }} 人</span>
          </template>
        </Column>
        <Column v-if="canAnyAction" header="操作" style="width: 170px">
          <template #body="{ data }">
            <div class="row-actions">
              <Button
                v-if="canEdit"
                label="编辑"
                text
                size="small"
                class="row-action"
                @click.stop="edit(data)"
              />
              <Button
                v-if="canCreate"
                label="克隆"
                text
                size="small"
                class="row-action"
                @click.stop="openClone(data)"
              />
              <Button
                v-if="canDelete"
                label="删除"
                text
                size="small"
                severity="danger"
                class="row-action"
                @click.stop="remove(data)"
              />
            </div>
          </template>
        </Column>
        <template #empty>
          <AppEmptyState
            icon="pi pi-trophy"
            title="暂无比赛"
            description="调整筛选条件，或等待新的比赛发布。"
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

    <Dialog
      v-model:visible="cloneDialog"
      header="克隆比赛"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 420px)' }"
      @hide="resetClone"
    >
      <AppFormField
        v-slot="{ id, invalid, describedby }"
        label="新比赛标题"
        :error="cloneError"
        required
      >
        <InputText
          :id="id"
          v-model="cloneTitle"
          fluid
          :invalid="invalid"
          :aria-describedby="describedby"
          @update:model-value="cloneError = ''"
        />
      </AppFormField>
      <template #footer>
        <Button label="取消" severity="secondary" text @click="cloneDialog = false" />
        <Button
          label="克隆"
          icon="pi pi-copy"
          :loading="cloning"
          class="clone-submit"
          @click="submitClone"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.contests-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.contests-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.notice-chip {
  padding: 3px var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}

.result-count {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.filter-bar {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.filter-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--kapoj-space-2);
  align-items: end;
}

.filter-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.filter-field label,
.filter-check label {
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.filter-check {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  min-height: 36px;
}

.filter-check label {
  cursor: pointer;
}

.filter-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
}

.contests-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.contests-table :deep(.p-datatable-table) {
  min-width: 830px;
}

.contests-table :deep(.p-datatable-thead > tr > th) {
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

.contests-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.contests-table :deep(.p-datatable-tbody > tr) {
  cursor: pointer;
}

.title-line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.contest-title {
  color: var(--kapoj-text);
  font-weight: 600;
}

.sub,
.time {
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.sub {
  margin-top: 2px;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
}

.row-action {
  padding: 2px 6px;
}

.pager {
  align-self: flex-end;
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
</style>
