<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import { useToast } from 'primevue/usetoast'

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
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useTheme } from '@/theme/theme'
import { CONTEST_MODES, CONTEST_REGISTER_TYPES } from '@/utils/contest'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const { isDark } = useTheme()

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

const form = ref<FormState>(emptyForm())
const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const problemOptions = ref<{ value: number; label: string }[]>([])
const problemLoading = ref(false)
const originalStart = ref<Date | null>(null)
const started = computed(
  () => originalStart.value !== null && originalStart.value.getTime() <= Date.now(),
)
const publicLabel = computed(() => (form.value.isPublic ? '公开' : '非公开'))

function warn(summary: string) {
  toast.add({ severity: 'warn', summary, life: 3000 })
}

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

function onFilterProblems(event: { value: string }) {
  void searchProblems(event.value)
}

async function loadContest() {
  if (!isEdit.value) return
  loading.value = true
  loadError.value = ''
  try {
    const c = await getContest(contestId.value!)
    Object.assign(form.value, {
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
  const used = new Set(form.value.problems.map((p) => p.displayId))
  for (let i = 0; i < 26; i++) {
    const id = String.fromCharCode(65 + i)
    if (!used.has(id)) return id
  }
  return null
}

function addProblem() {
  if (form.value.problems.length >= 26) {
    warn('比赛最多 26 道题')
    return
  }
  const displayId = nextDisplayId()
  if (!displayId) return
  const used = new Set(form.value.problems.map((p) => p.problemId))
  const candidate = problemOptions.value.find((o) => !used.has(o.value))
  form.value.problems.push({ problemId: candidate?.value ?? 0, displayId, score: 100 })
}

function moveProblem(index: number, delta: -1 | 1) {
  const target = index + delta
  if (target < 0 || target >= form.value.problems.length) return
  const rows = form.value.problems
  ;[rows[index], rows[target]] = [rows[target]!, rows[index]!]
}

function removeProblem(index: number) {
  form.value.problems.splice(index, 1)
}

function normalizeDisplay(index: number) {
  const row = form.value.problems[index]
  if (!row) return
  row.displayId = row.displayId
    .toUpperCase()
    .replace(/[^A-Z]/g, '')
    .slice(0, 1)
}

async function save() {
  const current = form.value
  if (!current.title.trim()) {
    warn('请填写比赛标题')
    return
  }
  const { startTime, endTime } = current
  if (!startTime || !endTime) {
    warn('请选择开始和结束时间')
    return
  }
  if (startTime >= endTime) {
    warn('结束时间必须晚于开始时间')
    return
  }
  if (current.problems.some((p) => !p.problemId || !p.displayId)) {
    warn('请为每道题选择题目并填写大写编号')
    return
  }

  const input: ContestInput = {
    title: current.title.trim(),
    descriptionMd: current.descriptionMd,
    startTime: startTime.toISOString(),
    endTime: endTime.toISOString(),
    mode: current.mode,
    isRated: current.isRated,
    isPublic: current.isPublic,
    registerType: current.registerType,
    password: current.password,
    inviteCode: current.inviteCode,
    problems: current.problems.map((p) => ({ ...p })),
  }

  saving.value = true
  try {
    const saved = isEdit.value
      ? await updateContest(contestId.value!, input)
      : await createContest(input)
    toast.add({
      severity: 'success',
      summary: isEdit.value ? '比赛已更新' : '比赛已创建',
      life: 3000,
    })
    await router.push({ name: 'contest-detail', params: { id: saved.id } })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: e instanceof ApiError ? e.message : '保存失败',
      life: 4000,
    })
  } finally {
    saving.value = false
  }
}

function cancel() {
  router.back()
}

onMounted(() => {
  void loadProblems()
  void loadContest()
})
</script>

<template>
  <div class="contest-editor">
    <AppSurface v-if="loadError" padding="lg" class="state-surface">
      <AppErrorState :description="loadError" retry-label="重新加载" @retry="loadContest" />
      <RouterLink :to="{ name: 'contests' }" class="back-link">
        <Button label="返回比赛列表" icon="pi pi-arrow-left" text />
      </RouterLink>
    </AppSurface>

    <AppLoadingState v-else-if="loading && isEdit" label="正在加载比赛" />

    <template v-else>
      <AppPageHeader
        :eyebrow="isEdit ? 'CONTEST // EDIT' : 'CONTEST // NEW'"
        :title="isEdit ? '编辑比赛' : '新建比赛'"
        description="配置赛程、报名方式、比赛选项与题目列表。"
      >
        <template #actions>
          <Button label="取消" severity="secondary" text icon="pi pi-times" @click="cancel" />
          <Button
            label="保存"
            icon="pi pi-check"
            :loading="saving"
            class="save-contest"
            @click="save"
          />
        </template>
      </AppPageHeader>

      <AppSurface padding="lg" class="editor-surface">
        <div class="editor-grid">
          <AppFormField
            v-slot="{ id }"
            class="span-2"
            label="标题"
            required
            :hint="`${form.title.length}/100`"
          >
            <InputText :id="id" v-model="form.title" maxlength="100" fluid placeholder="比赛标题" />
          </AppFormField>

          <AppFormField v-slot="{ id }" label="开始时间" required>
            <DatePicker
              v-model="form.startTime"
              :input-id="id"
              show-time
              show-seconds
              hour-format="24"
              date-format="yy-mm-dd"
              :disabled="started"
              fluid
            />
          </AppFormField>

          <AppFormField v-slot="{ id }" label="结束时间" required>
            <DatePicker
              v-model="form.endTime"
              :input-id="id"
              show-time
              show-seconds
              hour-format="24"
              date-format="yy-mm-dd"
              fluid
            />
          </AppFormField>

          <AppFormField v-slot="{ id }" label="赛制">
            <Select
              v-model="form.mode"
              :input-id="id"
              :options="CONTEST_MODES"
              option-label="label"
              option-value="value"
              :disabled="started"
              fluid
            />
          </AppFormField>

          <AppFormField v-slot="{ id }" label="报名方式">
            <Select
              v-model="form.registerType"
              :input-id="id"
              :options="CONTEST_REGISTER_TYPES"
              option-label="label"
              option-value="value"
              fluid
            />
          </AppFormField>

          <AppFormField v-if="form.registerType === 'password'" v-slot="{ id }" label="比赛密码">
            <Password
              v-model="form.password"
              :input-id="id"
              :feedback="false"
              toggle-mask
              fluid
              :placeholder="isEdit ? '留空保留原密码' : '4–64 字节'"
            />
          </AppFormField>

          <AppFormField v-if="form.registerType === 'invite'" v-slot="{ id }" label="邀请码">
            <InputText
              :id="id"
              v-model="form.inviteCode"
              fluid
              :placeholder="isEdit ? '留空保留原邀请码' : '4–64 字节'"
            />
          </AppFormField>

          <div class="toggle-block span-2">
            <span class="toggle-label">选项</span>
            <div class="toggle-row">
              <span class="toggle-field">
                <ToggleSwitch v-model="form.isPublic" input-id="contest-public" />
                <label for="contest-public">{{ publicLabel }}</label>
              </span>
              <span class="toggle-field">
                <ToggleSwitch v-model="form.isRated" input-id="contest-rated" />
                <label for="contest-rated">计 Rating</label>
              </span>
            </div>
          </div>

          <AppFormField class="span-2" label="描述">
            <MarkdownEditor v-model="form.descriptionMd" :theme="isDark ? 'dark' : 'light'" />
          </AppFormField>
        </div>
      </AppSurface>

      <AppSurface padding="lg" class="problems-surface">
        <div class="section-head">
          <h2>比赛题目</h2>
          <Button
            label="添加题目"
            icon="pi pi-plus"
            size="small"
            severity="secondary"
            outlined
            :disabled="started"
            @click="addProblem"
          />
        </div>

        <DataTable
          :value="form.problems"
          size="small"
          responsive-layout="scroll"
          class="problems-editor-table"
        >
          <Column header="编号" style="width: 90px">
            <template #body="{ data, index }">
              <InputText
                v-model="data.displayId"
                maxlength="1"
                class="display-input"
                :disabled="started"
                @update:model-value="normalizeDisplay(index)"
              />
            </template>
          </Column>
          <Column header="题目" style="min-width: 260px">
            <template #body="{ data }">
              <Select
                v-model="data.problemId"
                :options="problemOptions"
                option-label="label"
                option-value="value"
                filter
                :loading="problemLoading"
                :disabled="started"
                placeholder="搜索题目标题或 ID"
                fluid
                @filter="onFilterProblems"
              />
            </template>
          </Column>
          <Column header="分数" style="width: 140px">
            <template #body="{ data }">
              <InputNumber
                v-model="data.score"
                :min="1"
                :max="1000"
                :show-buttons="true"
                :disabled="started"
                size="small"
                fluid
              />
            </template>
          </Column>
          <Column header="操作" style="width: 180px">
            <template #body="{ index }">
              <div class="row-actions">
                <Button
                  label="上移"
                  text
                  size="small"
                  :disabled="started || index === 0"
                  @click="moveProblem(index, -1)"
                />
                <Button
                  label="下移"
                  text
                  size="small"
                  :disabled="started || index === form.problems.length - 1"
                  @click="moveProblem(index, 1)"
                />
                <Button
                  label="删除"
                  text
                  size="small"
                  severity="danger"
                  :disabled="started"
                  @click="removeProblem(index)"
                />
              </div>
            </template>
          </Column>
          <template #empty>
            <AppEmptyState
              icon="pi pi-list"
              title="还没有题目"
              description="点击“添加题目”开始配置比赛题目。"
            />
          </template>
        </DataTable>

        <p class="hint">
          {{
            started
              ? '比赛已开始，开始时间、赛制与题目列表已冻结。'
              : '题目编号为单个大写字母 A–Z，最多 26 题。'
          }}
        </p>
      </AppSurface>
    </template>
  </div>
</template>

<style scoped>
.contest-editor {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.state-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  align-items: flex-start;
}

.back-link {
  color: var(--kapoj-primary);
  font-size: 0.875rem;
}

.editor-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-4);
}

.span-2 {
  grid-column: 1 / -1;
}

.toggle-block {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
}

.toggle-label {
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.toggle-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-5);
}

.toggle-field {
  display: inline-flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.toggle-field label {
  color: var(--kapoj-text);
  font-size: 13px;
  cursor: pointer;
}

.section-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  margin-bottom: var(--kapoj-space-3);
}

.section-head h2 {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
}

.problems-editor-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.problems-editor-table :deep(.p-datatable-thead > tr > th) {
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

.problems-editor-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.display-input {
  width: 3.5rem;
  text-align: center;
  text-transform: uppercase;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
}

.hint {
  margin: var(--kapoj-space-3) 0 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

@media (max-width: 800px) {
  .editor-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
