<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'

import { createContest, getContest, updateContest } from '@/api/contests'
import { ApiError } from '@/api/http'
import { listProblems } from '@/api/problems'
import type {
  ContestInput,
  ContestMode,
  ContestProblemInput,
  ContestRegisterType,
  ProblemListItem,
} from '@/api/types'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import { CONTEST_MODES, CONTEST_REGISTER_TYPES } from '@/utils/contest'

const route = useRoute()
const router = useRouter()
const contestId = computed(() => {
  const raw = Array.isArray(route.params.id) ? route.params.id[0] : route.params.id
  const n = Number(raw)
  return Number.isInteger(n) && n > 0 ? n : null
})
const isEdit = computed(() => contestId.value !== null)

interface FormState {
  title: string
  descriptionMd: string
  startTime: Date | null
  endTime: Date | null
  mode: ContestMode
  isRated: boolean
  isPublic: boolean
  registerType: ContestRegisterType
  password: string
  inviteCode: string
  problems: ContestProblemInput[]
}

function defaultTimes(): { start: Date; end: Date } {
  const start = new Date(Date.now() + 60 * 60 * 1000)
  start.setSeconds(0, 0)
  const end = new Date(start.getTime() + 2 * 60 * 60 * 1000)
  return { start, end }
}

function emptyForm(): FormState {
  const { start, end } = defaultTimes()
  return {
    title: '',
    descriptionMd: '',
    startTime: start,
    endTime: end,
    mode: 'icpc',
    isRated: false,
    isPublic: true,
    registerType: 'open',
    password: '',
    inviteCode: '',
    problems: [],
  }
}

const form = reactive<FormState>(emptyForm())
const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const problemOptions = ref<{ value: number; label: string }[]>([])
const problemLoading = ref(false)
const originalStart = ref<Date | null>(null)
const started = computed(
  () => originalStart.value !== null && originalStart.value.getTime() <= Date.now(),
)

function addOptions(items: { problemId: number; title: string }[]) {
  for (const item of items) {
    if (!problemOptions.value.some((o) => o.value === item.problemId)) {
      problemOptions.value.push({ value: item.problemId, label: item.title })
    }
  }
}

async function loadProblems() {
  try {
    const page = await listProblems({ pageSize: 100 })
    addOptions(page.items.map((p: ProblemListItem) => ({ problemId: p.id, title: p.title })))
  } catch {
    // Keep any options already loaded from the contest.
  }
}

async function searchProblems(keyword: string) {
  problemLoading.value = true
  try {
    const page = await listProblems({ keyword, pageSize: 20 })
    addOptions(page.items.map((p: ProblemListItem) => ({ problemId: p.id, title: p.title })))
  } catch {
    // Ignore transient search failures; existing options remain usable.
  } finally {
    problemLoading.value = false
  }
}

async function loadContest() {
  if (!isEdit.value) return
  loading.value = true
  loadError.value = ''
  try {
    const c = await getContest(contestId.value!)
    Object.assign(form, {
      title: c.title,
      descriptionMd: c.descriptionMd,
      startTime: new Date(c.startTime),
      endTime: new Date(c.endTime),
      mode: c.mode,
      isRated: c.isRated,
      isPublic: c.isPublic,
      registerType: c.registerType,
      password: '',
      inviteCode: '',
      problems: c.problems.map((p) => ({
        problemId: p.problemId,
        displayId: p.displayId,
        score: p.score,
      })),
    })
    addOptions(c.problems.map((p) => ({ problemId: p.problemId, title: p.title })))
    originalStart.value = new Date(c.startTime)
  } catch (e) {
    loadError.value = e instanceof ApiError ? e.message : '加载比赛失败'
  } finally {
    loading.value = false
  }
}

function nextDisplayId(): string | null {
  const used = new Set(form.problems.map((p) => p.displayId))
  for (let i = 0; i < 26; i++) {
    const id = String.fromCharCode(65 + i)
    if (!used.has(id)) return id
  }
  return null
}

function addProblem() {
  if (form.problems.length >= 26) {
    ElMessage.warning('比赛最多 26 道题')
    return
  }
  const displayId = nextDisplayId()
  if (!displayId) return
  const used = new Set(form.problems.map((p) => p.problemId))
  const candidate = problemOptions.value.find((o) => !used.has(o.value))
  form.problems.push({ problemId: candidate?.value ?? 0, displayId, score: 100 })
}

function moveProblem(index: number, delta: -1 | 1) {
  const target = index + delta
  if (target < 0 || target >= form.problems.length) return
  ;[form.problems[index], form.problems[target]] = [form.problems[target], form.problems[index]]
}

function removeProblem(index: number) {
  form.problems.splice(index, 1)
}

function normalizeDisplay(index: number) {
  const row = form.problems[index]
  row.displayId = row.displayId
    .toUpperCase()
    .replace(/[^A-Z]/g, '')
    .slice(0, 1)
}

async function save() {
  if (!form.title.trim()) {
    ElMessage.warning('请填写比赛标题')
    return
  }
  if (!form.startTime || !form.endTime) {
    ElMessage.warning('请选择开始和结束时间')
    return
  }
  if (form.startTime >= form.endTime) {
    ElMessage.warning('结束时间必须晚于开始时间')
    return
  }
  if (form.problems.some((p) => !p.problemId || !p.displayId)) {
    ElMessage.warning('请为每道题选择题目并填写大写编号')
    return
  }
  const input: ContestInput = {
    title: form.title.trim(),
    descriptionMd: form.descriptionMd,
    startTime: form.startTime.toISOString(),
    endTime: form.endTime.toISOString(),
    mode: form.mode,
    isRated: form.isRated,
    isPublic: form.isPublic,
    registerType: form.registerType,
    password: form.password,
    inviteCode: form.inviteCode,
    problems: form.problems.map((p) => ({ ...p })),
  }
  saving.value = true
  try {
    const saved = isEdit.value
      ? await updateContest(contestId.value!, input)
      : await createContest(input)
    ElMessage.success(isEdit.value ? '比赛已更新' : '比赛已创建')
    await router.push({ name: 'contest-detail', params: { id: saved.id } })
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void loadProblems()
  void loadContest()
})
</script>

<template>
  <div v-loading="loading" class="page">
    <el-result v-if="loadError" icon="warning" :title="loadError">
      <template #extra>
        <RouterLink :to="{ name: 'contests' }">
          <el-button type="primary">返回比赛列表</el-button>
        </RouterLink>
      </template>
    </el-result>

    <template v-else>
      <el-card shadow="never">
        <template #header>
          <div class="head">
            <span>{{ isEdit ? '编辑比赛' : '新建比赛' }}</span>
            <div>
              <el-button @click="router.back()">取消</el-button>
              <el-button type="primary" :loading="saving" @click="save">保存</el-button>
            </div>
          </div>
        </template>

        <el-form label-width="100px">
          <el-form-item label="标题">
            <el-input v-model="form.title" maxlength="100" show-word-limit />
          </el-form-item>
          <el-form-item label="开始时间">
            <el-date-picker v-model="form.startTime" type="datetime" :disabled="started" />
          </el-form-item>
          <el-form-item label="结束时间">
            <el-date-picker v-model="form.endTime" type="datetime" />
          </el-form-item>
          <el-form-item label="赛制">
            <el-select v-model="form.mode" style="width: 160px" :disabled="started">
              <el-option
                v-for="item in CONTEST_MODES"
                :key="item.value"
                :label="item.label"
                :value="item.value"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="报名方式">
            <el-select v-model="form.registerType" style="width: 180px">
              <el-option
                v-for="item in CONTEST_REGISTER_TYPES"
                :key="item.value"
                :label="item.label"
                :value="item.value"
              />
            </el-select>
          </el-form-item>
          <el-form-item v-if="form.registerType === 'password'" label="比赛密码">
            <el-input
              v-model="form.password"
              type="password"
              show-password
              :placeholder="isEdit ? '留空保留原密码' : '4–64 字节'"
            />
          </el-form-item>
          <el-form-item v-if="form.registerType === 'invite'" label="邀请码">
            <el-input
              v-model="form.inviteCode"
              :placeholder="isEdit ? '留空保留原邀请码' : '4–64 字节'"
            />
          </el-form-item>
          <el-form-item label="选项">
            <el-switch v-model="form.isPublic" active-text="公开" inactive-text="非公开" />
            <span class="spacer" />
            <el-switch v-model="form.isRated" active-text="计 Rating" />
          </el-form-item>
          <el-form-item label="描述">
            <MarkdownEditor v-model="form.descriptionMd" />
          </el-form-item>
        </el-form>
      </el-card>

      <el-card shadow="never" class="problems-card">
        <template #header>
          <div class="head">
            <span>比赛题目</span>
            <el-button size="small" :disabled="started" @click="addProblem">添加题目</el-button>
          </div>
        </template>
        <el-table :data="form.problems" row-key="displayId" empty-text="还没有题目">
          <el-table-column label="编号" width="90">
            <template #default="{ row, $index }">
              <el-input
                v-model="row.displayId"
                maxlength="1"
                style="width: 55px"
                :disabled="started"
                @input="normalizeDisplay($index)"
              />
            </template>
          </el-table-column>
          <el-table-column label="题目" min-width="260">
            <template #default="{ row }">
              <el-select
                v-model="row.problemId"
                filterable
                remote
                :remote-method="searchProblems"
                :loading="problemLoading"
                :disabled="started"
                placeholder="搜索题目标题或 ID"
                style="width: 100%"
              >
                <el-option
                  v-for="item in problemOptions"
                  :key="item.value"
                  :label="item.label"
                  :value="item.value"
                />
              </el-select>
            </template>
          </el-table-column>
          <el-table-column label="分数" width="130">
            <template #default="{ row }">
              <el-input-number
                v-model="row.score"
                :min="1"
                :max="1000"
                size="small"
                :disabled="started"
              />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150">
            <template #default="{ $index }">
              <el-button
                text
                size="small"
                :disabled="$index === 0"
                @click="moveProblem($index, -1)"
              >
                上移
              </el-button>
              <el-button
                text
                size="small"
                :disabled="started || $index === form.problems.length - 1"
                @click="moveProblem($index, 1)"
              >
                下移
              </el-button>
              <el-button
                text
                size="small"
                type="danger"
                :disabled="started"
                @click="removeProblem($index)"
              >
                删除
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <p class="hint">
          {{
            started
              ? '比赛已开始，开始时间、赛制与题目列表已冻结。'
              : '题目编号为单个大写字母 A–Z，最多 26 题。'
          }}
        </p>
      </el-card>
    </template>
  </div>
</template>

<style scoped>
.page {
  min-height: 200px;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.spacer {
  display: inline-block;
  width: 24px;
}
.problems-card {
  margin-top: 16px;
}
.hint {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
</style>
