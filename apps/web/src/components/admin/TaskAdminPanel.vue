<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { createTask, deleteTask, listTasks, updateTask } from '@/api/gamification'
import { ApiError } from '@/api/http'
import { getProblem, listProblems } from '@/api/problems'
import type { ProblemListItem, TaskInput, TaskItem, TaskStatus } from '@/api/types'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import { formatDateTime } from '@/utils/format'
import { formatCoins, taskStatusLabel } from '@/utils/gamification'
import { useTheme } from '@/theme/theme'

interface ProblemOption {
  value: number
  label: string
}

interface FormState {
  title: string
  descriptionMd: string
  problemIds: number[]
  rewardCoins: number
  startTime: Date | null
  endTime: Date | null
  isActive: boolean
}

const MAX_PROBLEMS = 20
const toast = useToast()
const confirm = useConfirm()
const { isDark } = useTheme()

const tasks = ref<TaskItem[]>([])
const loading = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const saving = ref(false)
const deletingId = ref<number | null>(null)
const editing = ref<TaskItem | null>(null)
const problemOptions = ref<ProblemOption[]>([])
const problemLoading = ref(false)
const frozen = computed(() => editing.value?.hasRecords === true)

function defaultRange(): { start: Date; end: Date } {
  const start = new Date(Date.now() + 60 * 60 * 1000)
  start.setSeconds(0, 0)
  return { start, end: new Date(start.getTime() + 24 * 60 * 60 * 1000) }
}

function emptyForm(): FormState {
  const { start, end } = defaultRange()
  return {
    title: '',
    descriptionMd: '',
    problemIds: [],
    rewardCoins: 0,
    startTime: start,
    endTime: end,
    isActive: true,
  }
}

const form = ref<FormState>(emptyForm())

function upsertOption(id: number, title: string) {
  const label = `#${id} ${title}`
  const existing = problemOptions.value.find((option) => option.value === id)
  if (existing) {
    existing.label = label
    return
  }
  problemOptions.value.push({ value: id, label })
}

async function searchProblems(keyword: string) {
  problemLoading.value = true
  try {
    const page = await listProblems({ keyword: keyword.trim() || undefined, pageSize: 20 })
    for (const problem of page.items as ProblemListItem[]) {
      upsertOption(problem.id, problem.title)
    }
  } catch {
    // Transient search failures keep the already loaded options usable.
  } finally {
    problemLoading.value = false
  }
}

function onFilterProblems(event: { value: string }) {
  void searchProblems(event.value)
}

async function resolveProblemTitles(ids: number[]) {
  for (const id of ids) problemOptions.value.push({ value: id, label: `#${id}` })
  const results = await Promise.allSettled(ids.map((id) => getProblem(id)))
  results.forEach((result, index) => {
    if (result.status === 'fulfilled') upsertOption(ids[index]!, result.value.title)
  })
}

function onProblemsChange(value: number[]) {
  if (value.length > MAX_PROBLEMS) {
    form.value.problemIds = value.slice(0, MAX_PROBLEMS)
    toast.add({
      severity: 'warn',
      summary: `任务最多选择 ${MAX_PROBLEMS} 道题目`,
      life: 3000,
    })
  }
}

function fail(e: unknown, fallback: string) {
  toast.add({
    severity: 'error',
    summary: '操作失败',
    detail: e instanceof ApiError ? e.message : fallback,
    life: 4000,
  })
}

function warn(summary: string) {
  toast.add({ severity: 'warn', summary, life: 3000 })
}

function taskSeverity(status: TaskStatus): 'success' | 'warn' | 'danger' | 'info' | 'secondary' {
  if (status === 'active' || status === 'completed') return 'success'
  if (status === 'claimed') return 'warn'
  if (status === 'expired') return 'danger'
  if (status === 'inactive') return 'secondary'
  return 'info'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    tasks.value = await listTasks({ includeInactive: true })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载任务失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  problemOptions.value = []
  form.value = emptyForm()
  dialogVisible.value = true
  void searchProblems('')
}

async function openEdit(item: TaskItem) {
  editing.value = item
  problemOptions.value = []
  form.value = {
    title: item.title,
    descriptionMd: item.descriptionMd,
    problemIds: [...item.problemIds],
    rewardCoins: item.rewardCoins,
    startTime: new Date(item.startTime),
    endTime: new Date(item.endTime),
    isActive: item.isActive,
  }
  // Resolve titles before the select renders so selected ids have labels.
  await resolveProblemTitles(item.problemIds)
  dialogVisible.value = true
  void searchProblems('')
}

function validate(): string | null {
  const current = form.value
  if (!current.title.trim()) return '请填写任务标题'
  if (current.problemIds.length < 1 || current.problemIds.length > MAX_PROBLEMS) {
    return `任务题目必须为 1-${MAX_PROBLEMS} 个`
  }
  if (!current.startTime || !current.endTime) return '请选择任务时间'
  if (current.startTime >= current.endTime) return '任务开始时间必须早于结束时间'
  if (
    !Number.isInteger(current.rewardCoins) ||
    current.rewardCoins < 0 ||
    current.rewardCoins > 1_000_000
  ) {
    return '任务奖励金币必须为 0-1,000,000 的整数'
  }
  return null
}

async function save() {
  const problem = validate()
  if (problem) {
    warn(problem)
    return
  }
  const current = form.value
  const input: TaskInput = {
    title: current.title.trim(),
    descriptionMd: current.descriptionMd,
    problemIds: [...current.problemIds],
    rewardCoins: current.rewardCoins,
    startTime: current.startTime!.toISOString(),
    endTime: current.endTime!.toISOString(),
    isActive: current.isActive,
  }

  saving.value = true
  try {
    if (editing.value) {
      await updateTask(editing.value.id, input)
      toast.add({ severity: 'success', summary: '任务已更新', life: 3000 })
    } else {
      await createTask(input)
      toast.add({ severity: 'success', summary: '任务已创建', life: 3000 })
    }
    dialogVisible.value = false
    await load()
  } catch (e) {
    fail(e, '保存任务失败')
  } finally {
    saving.value = false
  }
}

function remove(item: TaskItem) {
  confirm.require({
    header: '删除任务',
    message: `确定删除任务「${item.title}」吗？删除后任务将下线，已领取的用户仍可在截止前完成。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void doRemove(item)
    },
  })
}

async function doRemove(item: TaskItem) {
  deletingId.value = item.id
  try {
    await deleteTask(item.id)
    toast.add({ severity: 'success', summary: '任务已删除', life: 3000 })
    await load()
  } catch (e) {
    fail(e, '删除任务失败')
  } finally {
    deletingId.value = null
  }
}

onMounted(load)
</script>

<template>
  <div class="admin-panel">
    <div class="toolbar">
      <span class="count">共 {{ tasks.length }} 个任务</span>
      <Button label="新建任务" icon="pi pi-plus" size="small" @click="openCreate" />
    </div>

    <AppErrorState v-if="error && !tasks.length" :description="error" @retry="load" />
    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <DataTable
        :value="tasks"
        :loading="loading"
        data-key="id"
        size="small"
        responsive-layout="scroll"
        class="admin-table"
      >
        <Column header="标题" style="min-width: 160px">
          <template #body="{ data }">{{ data.title }}</template>
        </Column>
        <Column header="时间" style="min-width: 200px">
          <template #body="{ data }">
            <div class="time-cell">
              <span>开始 {{ formatDateTime(data.startTime) }}</span>
              <span>截止 {{ formatDateTime(data.endTime) }}</span>
            </div>
          </template>
        </Column>
        <Column header="奖励" style="width: 120px">
          <template #body="{ data }">金币 {{ formatCoins(data.rewardCoins) }}</template>
        </Column>
        <Column header="题目数" style="width: 90px">
          <template #body="{ data }"
            ><span class="num">{{ data.problemCount }}</span></template
          >
        </Column>
        <Column header="状态" style="width: 90px">
          <template #body="{ data }">
            <Tag :value="taskStatusLabel(data.status)" :severity="taskSeverity(data.status)" />
          </template>
        </Column>
        <Column header="领取记录" style="width: 100px">
          <template #body="{ data }">
            <Tag
              :value="data.hasRecords ? '已有领取' : '无'"
              :severity="data.hasRecords ? 'warn' : 'secondary'"
            />
          </template>
        </Column>
        <Column header="操作" style="width: 150px">
          <template #body="{ data }">
            <div class="row-actions">
              <Button label="编辑" text size="small" @click="openEdit(data)" />
              <Button
                label="删除"
                text
                size="small"
                severity="danger"
                :loading="deletingId === data.id"
                @click="remove(data)"
              />
            </div>
          </template>
        </Column>
        <template #empty><AppEmptyState title="暂无任务" /></template>
      </DataTable>
    </template>

    <Dialog
      v-model:visible="dialogVisible"
      :header="editing ? '编辑任务' : '新建任务'"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 760px)' }"
    >
      <Message v-if="frozen" severity="warn" :closable="false" class="freeze-hint">
        已有领取记录，题目与奖励已冻结
      </Message>

      <div class="admin-form">
        <div class="field span-2">
          <label for="task-title">标题 <span class="required">*</span></label>
          <InputText
            id="task-title"
            v-model="form.title"
            class="task-title-input"
            maxlength="100"
            fluid
          />
        </div>

        <div class="field span-2">
          <label for="task-problems">题目 <span class="required">*</span></label>
          <MultiSelect
            v-model="form.problemIds"
            input-id="task-problems"
            class="task-problems-select"
            :options="problemOptions"
            option-label="label"
            option-value="value"
            filter
            display="chip"
            :loading="problemLoading"
            :disabled="frozen"
            placeholder="输入题目标题搜索"
            fluid
            @filter="onFilterProblems"
            @update:model-value="onProblemsChange"
          />
          <span class="hint">最多选择 20 道题目；已选 {{ form.problemIds.length }} 道</span>
        </div>

        <div class="field span-2">
          <span class="field-label">描述</span>
          <MarkdownEditor
            v-model="form.descriptionMd"
            height="260px"
            :theme="isDark ? 'dark' : 'light'"
          />
        </div>

        <div class="field">
          <label for="task-reward">奖励金币</label>
          <InputNumber
            v-model="form.rewardCoins"
            input-id="task-reward"
            :min="0"
            :max="1000000"
            :step="1"
            :disabled="frozen"
            fluid
          />
        </div>

        <div class="field">
          <label for="task-start">开始时间 <span class="required">*</span></label>
          <DatePicker
            v-model="form.startTime"
            input-id="task-start"
            show-time
            show-seconds
            hour-format="24"
            date-format="yy-mm-dd"
            fluid
          />
        </div>
        <div class="field">
          <label for="task-end">结束时间 <span class="required">*</span></label>
          <DatePicker
            v-model="form.endTime"
            input-id="task-end"
            show-time
            show-seconds
            hour-format="24"
            date-format="yy-mm-dd"
            fluid
          />
        </div>

        <div class="field toggle-field">
          <span class="field-label">上架</span>
          <div class="toggle-row">
            <ToggleSwitch v-model="form.isActive" input-id="task-active" />
            <label for="task-active">{{ form.isActive ? '已上架' : '已下线' }}</label>
          </div>
        </div>
      </div>

      <template #footer>
        <Button label="取消" severity="secondary" text @click="dialogVisible = false" />
        <Button label="保存" icon="pi pi-check" :loading="saving" @click="save" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.admin-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.count {
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.admin-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.admin-table :deep(.p-datatable-thead > tr > th) {
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

.admin-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.time-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-1);
}

.admin-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  min-width: 0;
}

.span-2 {
  grid-column: 1 / -1;
}

.field label,
.field-label {
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  font-weight: 600;
}

.required {
  color: var(--kapoj-danger);
}

.hint {
  color: var(--kapoj-text-muted);
  font-size: 11px;
  line-height: 1.4;
}

.freeze-hint {
  margin-bottom: var(--kapoj-space-3);
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.toggle-row label {
  color: var(--kapoj-text);
  cursor: pointer;
}

@media (max-width: 640px) {
  .admin-form {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
