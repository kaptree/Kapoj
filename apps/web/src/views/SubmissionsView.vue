<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'

import { ApiError } from '@/api/http'
import { getSubmission, listSubmissions } from '@/api/submissions'
import type {
  SubmissionDetail,
  SubmissionLanguage,
  SubmissionListItem,
  SubmissionStatus,
} from '@/api/types'
import { formatDateTime } from '@/utils/format'
import {
  languageLabel,
  statusLabel,
  statusTagType,
  SUBMISSION_LANGUAGES,
  SUBMISSION_STATUSES,
} from '@/utils/submission'

const PAGE_SIZE = 20
const route = useRoute()
const router = useRouter()

interface FilterState {
  problemId: number | undefined
  contestId: number | undefined
  userId: number | undefined
  language: SubmissionLanguage | ''
  status: SubmissionStatus | ''
  nickname: string
}

function emptyFilters(): FilterState {
  return {
    problemId: undefined,
    contestId: undefined,
    userId: undefined,
    language: '',
    status: '',
    nickname: '',
  }
}

// Draft filters are bound to the form; applied filters come from the URL and
// are the only ones used by background SSE/pagination reloads.
const filters = reactive<FilterState>(emptyFilters())
const applied = reactive<FilterState & { page: number }>({ ...emptyFilters(), page: 1 })
const items = ref<SubmissionListItem[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')
const live = ref(false)

const drawer = ref(false)
const detail = ref<SubmissionDetail | null>(null)
const detailLoading = ref(false)

let latest = 0
let detailSeq = 0
let source: EventSource | null = null
let reloadTimer: number | undefined
let syncTimer: number | undefined

function first(v: unknown): string {
  const raw = Array.isArray(v) ? v[0] : v
  return typeof raw === 'string' ? raw : ''
}

function numberQuery(v: unknown): number | undefined {
  const raw = first(v)
  if (raw === '') return undefined
  const n = Number(raw)
  return Number.isInteger(n) && n >= 0 ? n : undefined
}

function applyQuery() {
  const parsed: FilterState = {
    problemId: numberQuery(route.query.problemId),
    contestId: numberQuery(route.query.contestId),
    userId: numberQuery(route.query.userId),
    language: (first(route.query.language) as SubmissionLanguage | '') || '',
    status: (first(route.query.status) as SubmissionStatus | '') || '',
    nickname: first(route.query.nickname),
  }
  Object.assign(filters, parsed)
  Object.assign(applied, parsed)
  applied.page = numberQuery(route.query.page) ?? 1
}

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listSubmissions({
      problemId: applied.problemId,
      contestId: applied.contestId,
      userId: applied.userId,
      language: applied.language,
      status: applied.status,
      nickname: applied.nickname,
      page: applied.page,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latest) return
    items.value = data.items
    total.value = data.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载测评记录失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function queryFrom(state: FilterState, nextPage: number): Record<string, string> {
  const q: Record<string, string> = {}
  if (state.problemId != null) q.problemId = String(state.problemId)
  if (state.contestId != null) q.contestId = String(state.contestId)
  if (state.userId != null) q.userId = String(state.userId)
  if (state.language) q.language = state.language
  if (state.status) q.status = state.status
  if (state.nickname.trim()) q.nickname = state.nickname.trim()
  if (nextPage > 1) q.page = String(nextPage)
  return q
}

function search() {
  void router.push({ name: 'submissions', query: queryFrom(filters, 1) })
}

function reset() {
  if (Object.keys(route.query).length === 0) {
    void load()
    return
  }
  void router.push({ name: 'submissions', query: {} })
}

function changePage(next: number) {
  void router.push({ name: 'submissions', query: queryFrom(applied, next) })
}

watch(
  () => route.query,
  () => {
    applyQuery()
    void load()
  },
  { immediate: true },
)

async function openDetail(row: SubmissionListItem) {
  const seq = ++detailSeq
  drawer.value = true
  detail.value = null
  detailLoading.value = true
  try {
    const next = await getSubmission(row.id)
    if (seq === detailSeq) detail.value = next
  } catch (e) {
    if (seq === detailSeq) {
      ElMessage.error(e instanceof ApiError ? e.message : '加载记录详情失败')
    }
  } finally {
    if (seq === detailSeq) detailLoading.value = false
  }
}

function scheduleReload() {
  if (reloadTimer !== undefined) return
  reloadTimer = window.setTimeout(() => {
    reloadTimer = undefined
    void load()
  }, 300)
}

function connectStream() {
  if (typeof EventSource === 'undefined') return
  source = new EventSource('/api/submissions/stream')
  source.onopen = () => {
    live.value = true
    scheduleReload()
  }
  source.onerror = () => {
    live.value = false
  }
  source.onmessage = () => scheduleReload()
}

onMounted(() => {
  connectStream()
  syncTimer = window.setInterval(() => void load(), 30000)
})
onBeforeUnmount(() => {
  source?.close()
  if (reloadTimer !== undefined) window.clearTimeout(reloadTimer)
  if (syncTimer !== undefined) window.clearInterval(syncTimer)
})
</script>

<template>
  <el-card shadow="never">
    <template #header>
      <div class="head">
        <span>测评记录</span>
        <el-tag :type="live ? 'success' : 'info'" size="small" effect="plain">
          {{ live ? '实时刷新' : '连接中断，正在重连' }}
        </el-tag>
      </div>
    </template>

    <el-form inline @submit.prevent="search">
      <el-form-item label="题目 ID">
        <el-input-number
          v-model="filters.problemId"
          :min="0"
          :controls="false"
          style="width: 110px"
        />
      </el-form-item>
      <el-form-item label="比赛 ID">
        <el-input-number
          v-model="filters.contestId"
          :min="0"
          :controls="false"
          style="width: 110px"
        />
      </el-form-item>
      <el-form-item label="用户 ID">
        <el-input-number v-model="filters.userId" :min="0" :controls="false" style="width: 110px" />
      </el-form-item>
      <el-form-item label="昵称">
        <el-input v-model="filters.nickname" clearable style="width: 160px" />
      </el-form-item>
      <el-form-item label="语言">
        <el-select v-model="filters.language" clearable style="width: 120px">
          <el-option
            v-for="item in SUBMISSION_LANGUAGES"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="状态">
        <el-select v-model="filters.status" clearable style="width: 130px">
          <el-option
            v-for="item in SUBMISSION_STATUSES"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </el-select>
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
      empty-text="暂无测评记录"
      @row-click="openDetail"
    >
      <el-table-column prop="id" label="#" width="80" />
      <el-table-column label="提交者" min-width="130">
        <template #default="{ row }">
          <RouterLink :to="{ name: 'user-profile', params: { id: row.userId } }" @click.stop>
            {{ row.nickname || row.username }}
          </RouterLink>
        </template>
      </el-table-column>
      <el-table-column label="题目" min-width="180">
        <template #default="{ row }">
          <RouterLink :to="{ name: 'problem-detail', params: { id: row.problemId } }" @click.stop>
            {{ row.problemTitle }}
          </RouterLink>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.status)" size="small">
            {{ statusLabel(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="语言" width="100">
        <template #default="{ row }">{{ languageLabel(row.language) }}</template>
      </el-table-column>
      <el-table-column label="耗时" width="90">
        <template #default="{ row }">{{ row.timeUsedMs }} ms</template>
      </el-table-column>
      <el-table-column label="内存" width="100">
        <template #default="{ row }">{{ row.memoryUsedKb }} KB</template>
      </el-table-column>
      <el-table-column label="得分" width="70" prop="score" />
      <el-table-column label="提交时间" min-width="170">
        <template #default="{ row }">{{ formatDateTime(row.createdAt) }}</template>
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

  <el-drawer v-model="drawer" title="测评详情" size="62%">
    <div v-loading="detailLoading">
      <template v-if="detail">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="提交">#{{ detail.id }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="statusTagType(detail.status)" size="small">
              {{ statusLabel(detail.status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="用户">{{
            detail.nickname || detail.username
          }}</el-descriptions-item>
          <el-descriptions-item label="题目">{{ detail.problemTitle }}</el-descriptions-item>
          <el-descriptions-item label="语言">{{
            languageLabel(detail.language)
          }}</el-descriptions-item>
          <el-descriptions-item label="得分">{{ detail.score }}</el-descriptions-item>
          <el-descriptions-item label="耗时">{{ detail.timeUsedMs }} ms</el-descriptions-item>
          <el-descriptions-item label="内存">{{ detail.memoryUsedKb }} KB</el-descriptions-item>
          <el-descriptions-item label="提交时间">{{
            formatDateTime(detail.createdAt)
          }}</el-descriptions-item>
          <el-descriptions-item label="评测时间">{{
            formatDateTime(detail.judgedAt)
          }}</el-descriptions-item>
        </el-descriptions>

        <el-alert
          v-if="detail.codeHidden"
          title="比赛进行中，其他选手的代码与测评详情暂不可见。"
          type="warning"
          show-icon
          :closable="false"
          class="banner"
        />
        <template v-else>
          <h3>代码</h3>
          <pre class="code">{{ detail.code }}</pre>
          <template v-if="detail.errorMessage">
            <h3>错误信息</h3>
            <pre class="code">{{ detail.errorMessage }}</pre>
          </template>
          <template v-if="detail.judgeResult.length">
            <h3>测试点</h3>
            <el-table :data="detail.judgeResult" size="small">
              <el-table-column prop="testcaseId" label="测试点" width="90" />
              <el-table-column label="状态" width="110">
                <template #default="{ row }">
                  <el-tag :type="statusTagType(row.status)" size="small">
                    {{ statusLabel(row.status) }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="timeMs" label="耗时 (ms)" width="110" />
              <el-table-column prop="memoryKb" label="内存 (KB)" width="110" />
              <el-table-column prop="message" label="信息" min-width="160" />
            </el-table>
          </template>
        </template>
      </template>
      <el-empty v-else-if="!detailLoading" description="暂无详情" />
    </div>
  </el-drawer>
</template>

<style scoped>
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.banner {
  margin-bottom: 12px;
}
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
.code {
  padding: 12px;
  overflow: auto;
  max-height: 420px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  white-space: pre-wrap;
}
h3 {
  margin: 20px 0 8px;
}
</style>
