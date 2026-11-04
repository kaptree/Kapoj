<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'

import { cloneContest, deleteContest, listContests } from '@/api/contests'
import { ApiError } from '@/api/http'
import type { ContestListItem, ContestMode, ContestStatus } from '@/api/types'
import { hasRole } from '@/router/guard'
import { useAuthStore } from '@/stores/auth'
import {
  contestModeLabel,
  contestStatusLabel,
  contestStatusTagType,
  CONTEST_MODES,
  CONTEST_STATUSES,
  registerTypeLabel,
} from '@/utils/contest'
import { formatDateTime } from '@/utils/format'

const PAGE_SIZE = 20
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const canManage = computed(() => hasRole(auth.user?.role ?? null, ['contest_admin']))

interface Filters {
  keyword: string
  mode: ContestMode | ''
  status: ContestStatus | ''
  mine: boolean
  includeHidden: boolean
}

function emptyFilters(): Filters {
  return { keyword: '', mode: '', status: '', mine: false, includeHidden: false }
}

const filters = reactive<Filters>(emptyFilters())
const applied = reactive<Filters & { page: number }>({ ...emptyFilters(), page: 1 })
const items = ref<ContestListItem[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')
let latest = 0

function first(value: unknown): string {
  const raw = Array.isArray(value) ? value[0] : value
  return typeof raw === 'string' ? raw : ''
}

function boolQuery(value: unknown): boolean {
  return first(value) === 'true'
}

function intQuery(value: unknown, fallback: number): number {
  const n = Number(first(value))
  return Number.isInteger(n) && n >= 1 ? n : fallback
}

function applyQuery() {
  const parsed: Filters = {
    keyword: first(route.query.keyword),
    mode: (first(route.query.mode) as ContestMode | '') || '',
    status: (first(route.query.status) as ContestStatus | '') || '',
    mine: boolQuery(route.query.mine),
    includeHidden: boolQuery(route.query.includeHidden),
  }
  Object.assign(filters, parsed)
  Object.assign(applied, parsed)
  applied.page = intQuery(route.query.page, 1)
}

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listContests({
      keyword: applied.keyword,
      mode: applied.mode,
      status: applied.status,
      mine: applied.mine,
      includeHidden: applied.includeHidden,
      page: applied.page,
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
  void router.push({ name: 'contests', query: queryFrom(filters, 1) })
}

function reset() {
  if (Object.keys(route.query).length === 0) {
    void load()
    return
  }
  void router.push({ name: 'contests', query: {} })
}

function changePage(page: number) {
  void router.push({ name: 'contests', query: queryFrom(applied, page) })
}

function openDetail(row: ContestListItem) {
  void router.push({ name: 'contest-detail', params: { id: row.id } })
}

function edit(row: ContestListItem) {
  void router.push({ name: 'contest-edit', params: { id: row.id } })
}

async function clone(row: ContestListItem) {
  try {
    const { value } = await ElMessageBox.prompt('新比赛标题', '克隆比赛', {
      inputValue: `${row.title}（副本）`,
      confirmButtonText: '克隆',
      cancelButtonText: '取消',
    })
    const created = await cloneContest(row.id, { title: value })
    ElMessage.success('比赛已克隆')
    await router.push({ name: 'contest-edit', params: { id: created.id } })
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e instanceof ApiError ? e.message : '克隆失败')
  }
}

async function remove(row: ContestListItem) {
  try {
    await ElMessageBox.confirm(`确定删除比赛「${row.title}」吗？`, '删除比赛', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteContest(row.id)
    ElMessage.success('已删除')
    await load()
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '删除失败')
  }
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
  <el-card shadow="never">
    <template #header>
      <div class="head">
        <span>比赛</span>
        <div class="actions">
          <el-tag v-if="auth.isLoggedIn" size="small" effect="plain">
            我报名的比赛可在此筛选
          </el-tag>
          <el-button
            v-if="canManage"
            type="primary"
            @click="router.push({ name: 'contest-create' })"
          >
            新建比赛
          </el-button>
        </div>
      </div>
    </template>

    <el-form inline @submit.prevent="search">
      <el-form-item label="关键词">
        <el-input v-model="filters.keyword" clearable placeholder="比赛标题" style="width: 200px" />
      </el-form-item>
      <el-form-item label="赛制">
        <el-select v-model="filters.mode" clearable style="width: 120px">
          <el-option
            v-for="item in CONTEST_MODES"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="状态">
        <el-select v-model="filters.status" clearable style="width: 120px">
          <el-option
            v-for="item in CONTEST_STATUSES"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </el-select>
      </el-form-item>
      <el-form-item v-if="auth.isLoggedIn">
        <el-checkbox v-model="filters.mine">只看我报名的</el-checkbox>
      </el-form-item>
      <el-form-item v-if="canManage">
        <el-checkbox v-model="filters.includeHidden">包含非公开</el-checkbox>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" native-type="submit">筛选</el-button>
        <el-button @click="reset">重置</el-button>
      </el-form-item>
    </el-form>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />

    <el-table
      v-loading="loading"
      :data="items"
      row-key="id"
      empty-text="暂无比赛"
      @row-click="openDetail"
    >
      <el-table-column label="比赛" min-width="200">
        <template #default="{ row }">
          <div class="title">
            <span>{{ row.title }}</span>
            <el-tag v-if="row.registered" type="success" size="small" effect="plain">已报名</el-tag>
          </div>
          <div class="sub">{{ registerTypeLabel(row.registerType) }}</div>
        </template>
      </el-table-column>
      <el-table-column label="赛制" width="70">
        <template #default="{ row }">{{ contestModeLabel(row.mode) }}</template>
      </el-table-column>
      <el-table-column label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="contestStatusTagType(row.status)" size="small">
            {{ contestStatusLabel(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="比赛时间" min-width="190">
        <template #default="{ row }">
          <div class="time">{{ formatDateTime(row.startTime) }}</div>
          <div class="time">至 {{ formatDateTime(row.endTime) }}</div>
        </template>
      </el-table-column>
      <el-table-column label="规模" width="100">
        <template #default="{ row }"
          >{{ row.problemCount }} 题 / {{ row.participantCount }} 人</template
        >
      </el-table-column>
      <el-table-column v-if="canManage" label="操作" width="170">
        <template #default="{ row }">
          <el-button text size="small" @click.stop="edit(row)">编辑</el-button>
          <el-button text size="small" @click.stop="clone(row)">克隆</el-button>
          <el-button text size="small" type="danger" @click.stop="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination
        layout="prev, pager, next, total"
        :current-page="applied.page"
        :page-size="PAGE_SIZE"
        :total="total"
        @current-change="changePage"
      />
    </div>
  </el-card>
</template>

<style scoped>
.head,
.actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sub {
  margin-top: 4px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.banner {
  margin-bottom: 12px;
}
.time {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
}
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
</style>
