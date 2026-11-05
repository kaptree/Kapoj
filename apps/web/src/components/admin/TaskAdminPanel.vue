<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

import { createTask, deleteTask, listTasks, updateTask } from '@/api/gamification'
import { ApiError } from '@/api/http'
import { getProblem, listProblems } from '@/api/problems'
import type { ProblemListItem, TaskInput, TaskItem } from '@/api/types'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import { formatDateTime } from '@/utils/format'
import { formatCoins, taskStatusLabel, taskStatusTagType } from '@/utils/gamification'

interface ProblemOption {
  value: number
  label: string
}

interface FormState {
  title: string
  descriptionMd: string
  problemIds: number[]
  rewardCoins: number
  timeRange: [Date, Date] | null
  isActive: boolean
}

const MAX_PROBLEMS = 20

const tasks = ref<TaskItem[]>([])
const loading = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const saving = ref(false)
const deletingId = ref<number | null>(null)
const editing = ref<TaskItem | null>(null)
const frozen = computed(() => editing.value?.hasRecords === true)
const problemOptions = ref<ProblemOption[]>([])
const problemLoading = ref(false)

function defaultRange(): [Date, Date] {
  const start = new Date(Date.now() + 60 * 60 * 1000)
  start.setSeconds(0, 0)
  return [start, new Date(start.getTime() + 24 * 60 * 60 * 1000)]
}

function emptyForm(): FormState {
  return {
    title: '',
    descriptionMd: '',
    problemIds: [],
    rewardCoins: 0,
    timeRange: defaultRange(),
    isActive: true,
  }
}

const form = reactive<FormState>(emptyForm())

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

async function resolveProblemTitles(ids: number[]) {
  for (const id of ids) problemOptions.value.push({ value: id, label: `#${id}` })
  const results = await Promise.allSettled(ids.map((id) => getProblem(id)))
  results.forEach((result, index) => {
    if (result.status === 'fulfilled') upsertOption(ids[index], result.value.title)
  })
}

function onProblemsChange(value: number[]) {
  if (value.length > MAX_PROBLEMS) {
    form.problemIds = value.slice(0, MAX_PROBLEMS)
    ElMessage.warning(`任务最多选择 ${MAX_PROBLEMS} 道题目`)
  }
}

function fail(e: unknown, fallback: string) {
  ElMessage.error(e instanceof ApiError ? e.message : fallback)
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
  Object.assign(form, emptyForm())
  dialogVisible.value = true
  void searchProblems('')
}

async function openEdit(item: TaskItem) {
  editing.value = item
  problemOptions.value = []
  Object.assign(form, {
    title: item.title,
    descriptionMd: item.descriptionMd,
    problemIds: [...item.problemIds],
    rewardCoins: item.rewardCoins,
    timeRange: [new Date(item.startTime), new Date(item.endTime)],
    isActive: item.isActive,
  })
  // Resolve titles before the select renders, otherwise Element Plus caches
  // the temporary #id labels for the already selected values.
  await resolveProblemTitles(item.problemIds)
  dialogVisible.value = true
  void searchProblems('')
}

function validate(): string | null {
  if (!form.title.trim()) return '请填写任务标题'
  if (form.problemIds.length < 1 || form.problemIds.length > MAX_PROBLEMS) {
    return `任务题目必须为 1-${MAX_PROBLEMS} 个`
  }
  if (!form.timeRange || form.timeRange.length !== 2) return '请选择任务时间'
  if (form.timeRange[0] >= form.timeRange[1]) return '任务开始时间必须早于结束时间'
  if (!Number.isInteger(form.rewardCoins) || form.rewardCoins < 0 || form.rewardCoins > 1_000_000) {
    return '任务奖励金币必须为 0-1,000,000 的整数'
  }
  return null
}

async function save() {
  const problem = validate()
  if (problem) {
    ElMessage.warning(problem)
    return
  }
  const [start, end] = form.timeRange!
  const input: TaskInput = {
    title: form.title.trim(),
    descriptionMd: form.descriptionMd,
    problemIds: [...form.problemIds],
    rewardCoins: form.rewardCoins,
    startTime: start.toISOString(),
    endTime: end.toISOString(),
    isActive: form.isActive,
  }

  saving.value = true
  try {
    if (editing.value) {
      await updateTask(editing.value.id, input)
      ElMessage.success('任务已更新')
    } else {
      await createTask(input)
      ElMessage.success('任务已创建')
    }
    dialogVisible.value = false
    await load()
  } catch (e) {
    fail(e, '保存任务失败')
  } finally {
    saving.value = false
  }
}

async function remove(item: TaskItem) {
  try {
    await ElMessageBox.confirm(
      `确定删除任务「${item.title}」吗？删除后任务将下线，已领取的用户仍可在截止前完成。`,
      '删除任务',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  deletingId.value = item.id
  try {
    await deleteTask(item.id)
    ElMessage.success('任务已删除')
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
  <div>
    <div class="toolbar">
      <span class="count">共 {{ tasks.length }} 个任务</span>
      <el-button type="primary" @click="openCreate">新建任务</el-button>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />

    <el-table v-loading="loading" :data="tasks" empty-text="暂无任务">
      <el-table-column prop="title" label="标题" min-width="160" />
      <el-table-column label="时间" min-width="200">
        <template #default="{ row }">
          <div class="time-cell">
            <span>开始 {{ formatDateTime(row.startTime) }}</span>
            <span>截止 {{ formatDateTime(row.endTime) }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="奖励" width="120">
        <template #default="{ row }">金币 {{ formatCoins(row.rewardCoins) }}</template>
      </el-table-column>
      <el-table-column prop="problemCount" label="题目数" width="90" />
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="taskStatusTagType(row.status)" size="small">
            {{ taskStatusLabel(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="领取记录" width="100">
        <template #default="{ row }">
          <el-tag :type="row.hasRecords ? 'warning' : 'info'" size="small">
            {{ row.hasRecords ? '已有领取' : '无' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" :loading="deletingId === row.id" @click="remove(row)"
            >删除</el-button
          >
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      v-model="dialogVisible"
      :title="editing ? '编辑任务' : '新建任务'"
      width="760px"
      destroy-on-close
    >
      <el-alert
        v-if="frozen"
        title="已有领取记录，题目与奖励已冻结"
        type="warning"
        show-icon
        :closable="false"
        class="freeze-hint"
      />

      <el-form label-width="90px">
        <el-form-item label="标题" required>
          <el-input v-model="form.title" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item label="题目" required>
          <el-select
            v-model="form.problemIds"
            multiple
            filterable
            remote
            reserve-keyword
            :remote-method="searchProblems"
            :loading="problemLoading"
            :disabled="frozen"
            placeholder="输入题目标题搜索"
            style="width: 100%"
            @change="onProblemsChange"
          >
            <el-option
              v-for="option in problemOptions"
              :key="option.value"
              :label="option.label"
              :value="option.value"
            />
          </el-select>
          <div class="hint">最多选择 20 道题目；已选 {{ form.problemIds.length }} 道</div>
        </el-form-item>
        <el-form-item label="描述">
          <MarkdownEditor v-model="form.descriptionMd" height="260px" />
        </el-form-item>
        <el-form-item label="奖励金币">
          <el-input-number
            v-model="form.rewardCoins"
            :min="0"
            :max="1000000"
            :step="1"
            :disabled="frozen"
          />
        </el-form-item>
        <el-form-item label="任务时间" required>
          <el-date-picker
            v-model="form.timeRange"
            type="datetimerange"
            start-placeholder="开始时间"
            end-placeholder="结束时间"
            range-separator="至"
          />
        </el-form-item>
        <el-form-item label="上架">
          <el-switch v-model="form.isActive" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.count {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.banner {
  margin-bottom: 12px;
}
.time-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
}
.hint {
  width: 100%;
  margin-top: 4px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.freeze-hint {
  margin-bottom: 16px;
}
@media (max-width: 640px) {
  :deep(.el-dialog) {
    max-width: calc(100vw - 32px);
  }
}
</style>
