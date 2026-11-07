<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'

import { ApiError } from '@/api/http'
import { createLearningNode, updateLearningNode } from '@/api/learning'
import { listProblems } from '@/api/problems'
import type { LearningNodeInput, LearningPathNodeItem, ProblemListItem } from '@/api/types'
import AppFormField from '@/components/ui/AppFormField.vue'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import { useTheme } from '@/theme/theme'
import {
  descendantNodeIds,
  findLearningNode,
  flattenLearningNodes,
  siblingNodes,
} from '@/utils/learning'

const props = defineProps<{
  visible: boolean
  pathId: number
  nodes: LearningPathNodeItem[]
  editing: LearningPathNodeItem | null
  parentId: number | null
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  saved: []
}>()

const { isDark } = useTheme()

const dialogVisible = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value),
})

interface ProblemRow {
  problemId: number
  title: string
}

const form = reactive({
  title: '',
  descriptionMd: '',
  parentId: null as number | null,
  problems: [] as ProblemRow[],
  requiredCount: 1,
  sortOrder: 0,
})

const saving = ref(false)
const error = ref('')
const problemOptions = ref<{ value: number; label: string }[]>([])
const problemLoading = ref(false)
const pickedProblem = ref<number | null>(null)

const isEditing = computed(() => props.editing !== null)

const parentOptions = computed(() => {
  const excluded = props.editing ? descendantNodeIds(props.editing) : new Set<number>()
  const options: { value: number | null; label: string }[] = [
    { value: null, label: '无（根节点）' },
  ]
  for (const node of flattenLearningNodes(props.nodes)) {
    if (excluded.has(node.id)) continue
    options.push({ value: node.id, label: node.title })
  }
  return options
})

function addOption(id: number, title: string) {
  if (!problemOptions.value.some((option) => option.value === id)) {
    problemOptions.value.push({ value: id, label: title })
  }
}

function siblingsForParent(parentId: number | null): LearningPathNodeItem[] {
  if (parentId === null) return siblingNodes(props.nodes, null)
  const parent = findLearningNode(props.nodes, parentId)
  return siblingNodes(parent?.children ?? [], parentId)
}

function defaultSortOrder(): number {
  const siblings = siblingsForParent(props.parentId ?? null)
  return siblings.reduce((max, sibling) => Math.max(max, sibling.sortOrder), 0) + 1
}

function resetForm() {
  error.value = ''
  pickedProblem.value = null
  if (props.editing) {
    form.title = props.editing.title
    form.descriptionMd = props.editing.descriptionMd
    form.parentId = props.editing.parentId
    form.requiredCount = props.editing.requiredCount
    form.sortOrder = props.editing.sortOrder
    const byId = new Map(props.editing.problems.map((p) => [p.problemId, p.title]))
    form.problems = props.editing.problemIds.map((id) => ({
      problemId: id,
      title: byId.get(id) ?? `题目 #${id}`,
    }))
    for (const p of props.editing.problems) addOption(p.problemId, p.title)
  } else {
    form.title = ''
    form.descriptionMd = ''
    form.parentId = props.parentId ?? null
    form.requiredCount = 1
    form.problems = []
    form.sortOrder = defaultSortOrder()
  }
}

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    resetForm()
    void loadProblems()
  },
  { immediate: true },
)

async function loadProblems() {
  try {
    const page = await listProblems({ pageSize: 100 })
    for (const problem of page.items as ProblemListItem[]) addOption(problem.id, problem.title)
  } catch {
    // Existing options remain usable; a failed preload is harmless.
  }
}

async function searchProblems(keyword: string) {
  problemLoading.value = true
  try {
    const page = await listProblems({ keyword, pageSize: 20 })
    for (const problem of page.items as ProblemListItem[]) addOption(problem.id, problem.title)
  } catch {
    // Ignore transient search failures.
  } finally {
    problemLoading.value = false
  }
}

function onFilterProblems(event: { value: string }) {
  void searchProblems(event.value)
}

function onPickProblem(event: { value: number | null }) {
  const id = event.value
  if (id == null) return
  const option = problemOptions.value.find((item) => item.value === id)
  form.problems.push({ problemId: id, title: option?.label ?? `题目 #${id}` })
  pickedProblem.value = null
}

function removeProblem(index: number) {
  form.problems.splice(index, 1)
}

function validate(): string | null {
  const title = form.title.trim()
  if (title.length < 1 || title.length > 100) return '标题需为 1–100 个字符'
  if (form.descriptionMd.length > 50000) return '描述不能超过 50000 个字符'
  const ids = form.problems.map((problem) => problem.problemId)
  if (ids.length < 1) return '请至少添加一道题目'
  if (ids.length > 50) return '题目不能超过 50 道'
  if (new Set(ids).size !== ids.length) return '题目不能重复'
  const requiredCount = form.requiredCount
  if (!Number.isInteger(requiredCount) || requiredCount < 1 || requiredCount > ids.length) {
    return `完成题数需在 1–${ids.length} 之间`
  }
  if (props.editing && form.parentId !== null) {
    if (descendantNodeIds(props.editing).has(form.parentId)) {
      return '父节点不能选择自身或其子节点'
    }
  }
  return null
}

async function save() {
  error.value = ''
  const validationError = validate()
  if (validationError) {
    error.value = validationError
    return
  }

  saving.value = true
  try {
    const input: LearningNodeInput = {
      title: form.title.trim(),
      descriptionMd: form.descriptionMd,
      parentId: form.parentId,
      problemIds: form.problems.map((problem) => problem.problemId),
      sortOrder: form.sortOrder,
      requiredCount: form.requiredCount,
    }
    if (props.editing) {
      await updateLearningNode(props.pathId, props.editing.id, input)
    } else {
      await createLearningNode(props.pathId, input)
    }
    emit('saved')
    emit('update:visible', false)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '保存节点失败，请稍后再试'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog
    v-model:visible="dialogVisible"
    :header="isEditing ? '编辑节点' : '添加节点'"
    modal
    :draggable="false"
    :style="{ width: 'min(94vw, 640px)' }"
  >
    <div class="node-dialog">
      <Message v-if="error" severity="error" :closable="false" class="node-error" role="alert">
        {{ error }}
      </Message>

      <AppFormField v-slot="{ id }" label="标题" required>
        <InputText
          :id="id"
          v-model="form.title"
          class="node-title"
          maxlength="100"
          fluid
          placeholder="节点标题"
        />
      </AppFormField>

      <AppFormField label="父节点">
        <Select
          v-model="form.parentId"
          :options="parentOptions"
          option-label="label"
          option-value="value"
          fluid
          placeholder="选择父节点"
        />
      </AppFormField>

      <AppFormField label="描述">
        <MarkdownEditor
          v-model="form.descriptionMd"
          :theme="isDark ? 'dark' : 'light'"
          height="200px"
        />
      </AppFormField>

      <AppFormField label="题目" :hint="`${form.problems.length}/50，可搜索题目标题或 ID`">
        <Select
          v-model="pickedProblem"
          :options="problemOptions"
          option-label="label"
          option-value="value"
          filter
          :loading="problemLoading"
          placeholder="搜索题目并选择添加"
          fluid
          @filter="onFilterProblems"
          @change="onPickProblem"
        />

        <DataTable
          v-if="form.problems.length"
          :value="form.problems"
          size="small"
          responsive-layout="scroll"
          class="problem-table"
        >
          <Column header="题目" style="min-width: 240px">
            <template #body="{ data }">{{ data.title }}</template>
          </Column>
          <Column header="操作" style="width: 90px">
            <template #body="{ index }">
              <Button
                label="删除"
                text
                size="small"
                severity="danger"
                @click="removeProblem(index)"
              />
            </template>
          </Column>
        </DataTable>
        <p v-else class="problem-empty">尚未添加题目。</p>
      </AppFormField>

      <div class="node-row">
        <AppFormField label="完成题数" hint="1–题目数，默认等于题目数">
          <InputNumber v-model="form.requiredCount" input-id="node-required" :min="1" fluid />
        </AppFormField>
        <AppFormField label="排序">
          <InputNumber v-model="form.sortOrder" input-id="node-sort" :min="0" fluid />
        </AppFormField>
      </div>
    </div>

    <template #footer>
      <Button label="取消" severity="secondary" text @click="dialogVisible = false" />
      <Button label="保存" icon="pi pi-check" :loading="saving" @click="save" />
    </template>
  </Dialog>
</template>

<style scoped>
.node-dialog {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.node-error {
  width: 100%;
}

.node-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
}

.problem-table {
  margin-top: var(--kapoj-space-2);
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.problem-table :deep(.p-datatable-thead > tr > th) {
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

.problem-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.problem-empty {
  margin: var(--kapoj-space-2) 0 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

@media (max-width: 640px) {
  .node-row {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
