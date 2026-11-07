<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import TreeTable from 'primevue/treetable'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import type { TreeNode } from 'primevue/treenode'

import { ApiError } from '@/api/http'
import {
  createLearningPath,
  deleteLearningNode,
  deleteLearningPath,
  getLearningTree,
  updateLearningNode,
  updateLearningPath,
} from '@/api/learning'
import type {
  LearningNodeInput,
  LearningPathInput,
  LearningPathItem,
  LearningPathNodeItem,
} from '@/api/types'
import LearningNodeDialog from '@/components/learning/LearningNodeDialog.vue'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/theme/theme'
import {
  findLearningNode,
  flattenLearningNodes,
  learningStatusLabel,
  learningStatusSeverity,
  siblingNodes,
} from '@/utils/learning'
import { validateCoverUrl } from '@/validation'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const auth = useAuthStore()
const { isDark } = useTheme()

const pathId = computed<number | null>(() => {
  const raw = route.params.id
  const value = typeof raw === 'string' ? Number(raw) : NaN
  return Number.isInteger(value) && value > 0 ? value : null
})
const isEdit = computed(() => pathId.value !== null)

const canManageOfficial = computed(() => auth.can(PERM.learningPath.official.manage))
const showOfficialToggle = computed(() => !isEdit.value && canManageOfficial.value)

const form = reactive({
  title: '',
  descriptionMd: '',
  coverUrl: '',
  isPublic: false,
  isOfficial: false,
  sortOrder: 0,
})

const loading = ref(false)
const saving = ref(false)
const reordering = ref(false)
const loadError = ref('')
const path = ref<LearningPathItem | null>(null)
const nodes = ref<LearningPathNodeItem[]>([])
const expandedKeys = ref<Record<string, boolean>>({})

const nodeDialogVisible = ref(false)
const nodeEditing = ref<LearningPathNodeItem | null>(null)
const nodeParentId = ref<number | null>(null)

const publicModel = computed({
  get: () => form.isOfficial || form.isPublic,
  set: (value: boolean) => {
    form.isPublic = value
  },
})
const forcePublic = computed(() => form.isOfficial)

const treeNodes = computed<TreeNode[]>(() => nodes.value.map(toTreeNode))

function toTreeNode(node: LearningPathNodeItem): TreeNode {
  return {
    key: String(node.id),
    id: String(node.id),
    data: node,
    leaf: node.children.length === 0,
    children: node.children.map(toTreeNode),
  }
}

let latest = 0

function resetForm() {
  form.title = ''
  form.descriptionMd = ''
  form.coverUrl = ''
  form.isPublic = false
  form.isOfficial = route.query.official === 'true' && canManageOfficial.value
  form.sortOrder = 0
}

function fillForm(item: LearningPathItem) {
  form.title = item.title
  form.descriptionMd = item.descriptionMd
  form.coverUrl = item.coverUrl
  form.isPublic = item.isPublic
  form.isOfficial = item.isOfficial
  form.sortOrder = item.sortOrder
}

async function load() {
  const seq = ++latest
  loadError.value = ''
  if (!isEdit.value) {
    resetForm()
    loading.value = false
    return
  }
  loading.value = true
  try {
    const data = await getLearningTree(pathId.value!)
    if (seq !== latest) return
    path.value = data.path
    nodes.value = data.nodes
    fillForm(data.path)
    expandedKeys.value = Object.fromEntries(
      flattenLearningNodes(data.nodes).map((node) => [String(node.id), true]),
    )
  } catch (e) {
    if (seq !== latest) return
    loadError.value =
      e instanceof ApiError && e.code === 40400
        ? '学习路线不存在或无权查看'
        : '加载学习路线失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  () => route.params.id,
  () => {
    path.value = null
    nodes.value = []
    void load()
  },
  { immediate: true },
)

function cancel() {
  void router.push({ name: 'learning-paths' })
}

function validateForm(): string | null {
  const title = form.title.trim()
  if (title.length < 1 || title.length > 100) return '请填写标题（1–100 个字符）'
  const coverError = validateCoverUrl(form.coverUrl)
  if (coverError) return coverError
  return null
}

async function save() {
  const validationError = validateForm()
  if (validationError) {
    toast.add({ severity: 'warn', summary: validationError, life: 3000 })
    return
  }

  saving.value = true
  try {
    const input: LearningPathInput = {
      title: form.title.trim(),
      descriptionMd: form.descriptionMd,
      coverUrl: form.coverUrl.trim(),
      isPublic: form.isOfficial || form.isPublic,
      sortOrder: form.sortOrder,
    }
    if (showOfficialToggle.value) {
      input.isOfficial = form.isOfficial
    }

    if (isEdit.value) {
      await updateLearningPath(pathId.value!, input)
      toast.add({ severity: 'success', summary: '路线已更新', life: 3000 })
      await load()
    } else {
      const created = await createLearningPath(input)
      toast.add({ severity: 'success', summary: '路线已创建', life: 3000 })
      await router.replace({ name: 'learning-path-edit', params: { id: created.id } })
    }
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: e instanceof ApiError ? e.message : '保存失败，请稍后再试',
      life: 4000,
    })
  } finally {
    saving.value = false
  }
}

function confirmDelete() {
  confirm.require({
    header: '删除路线',
    message: '确定删除这条学习路线吗？删除后将从列表中移除。',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doDelete(),
  })
}

async function doDelete() {
  try {
    await deleteLearningPath(pathId.value!)
    toast.add({ severity: 'success', summary: '路线已删除', life: 3000 })
    await router.replace({ name: 'learning-paths' })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '删除失败',
      detail: e instanceof ApiError ? e.message : '删除失败，请稍后再试',
      life: 4000,
    })
  }
}

function openAddRoot() {
  nodeEditing.value = null
  nodeParentId.value = null
  nodeDialogVisible.value = true
}

function openAddChild(node: LearningPathNodeItem) {
  nodeEditing.value = null
  nodeParentId.value = node.id
  nodeDialogVisible.value = true
}

function openEdit(node: LearningPathNodeItem) {
  nodeEditing.value = node
  nodeParentId.value = null
  nodeDialogVisible.value = true
}

function onNodeSaved() {
  void load()
}

function siblingsOf(node: LearningPathNodeItem): LearningPathNodeItem[] {
  if (node.parentId === null) return siblingNodes(nodes.value, null)
  const parent = findLearningNode(nodes.value, node.parentId)
  return siblingNodes(parent?.children ?? [], node.parentId)
}

function canMoveUp(node: LearningPathNodeItem): boolean {
  const siblings = siblingsOf(node)
  return siblings.findIndex((sibling) => sibling.id === node.id) > 0
}

function canMoveDown(node: LearningPathNodeItem): boolean {
  const siblings = siblingsOf(node)
  const index = siblings.findIndex((sibling) => sibling.id === node.id)
  return index >= 0 && index < siblings.length - 1
}

function toNodeInput(node: LearningPathNodeItem, sortOrder: number): LearningNodeInput {
  return {
    title: node.title,
    descriptionMd: node.descriptionMd,
    parentId: node.parentId,
    problemIds: [...node.problemIds],
    sortOrder,
    requiredCount: node.requiredCount,
  }
}

async function moveNode(node: LearningPathNodeItem, delta: -1 | 1) {
  const siblings = siblingsOf(node)
  const index = siblings.findIndex((sibling) => sibling.id === node.id)
  if (index < 0) return
  const target = index + delta
  if (target < 0 || target >= siblings.length) return

  const reordered = [...siblings]
  ;[reordered[index], reordered[target]] = [reordered[target]!, reordered[index]!]
  const updates = reordered
    .map((sibling, i) => ({ node: sibling, sortOrder: i + 1 }))
    .filter(({ node: sibling, sortOrder }) => sibling.sortOrder !== sortOrder)

  reordering.value = true
  try {
    for (const { node: sibling, sortOrder } of updates) {
      await updateLearningNode(pathId.value!, sibling.id, toNodeInput(sibling, sortOrder))
    }
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '排序失败',
      detail: e instanceof ApiError ? e.message : '排序失败，请稍后再试',
      life: 4000,
    })
  } finally {
    reordering.value = false
  }
}

function confirmDeleteNode(node: LearningPathNodeItem) {
  confirm.require({
    header: '删除节点',
    message: `确定删除节点「${node.title}」吗？`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doDeleteNode(node),
  })
}

async function doDeleteNode(node: LearningPathNodeItem) {
  try {
    await deleteLearningNode(pathId.value!, node.id)
    toast.add({ severity: 'success', summary: '节点已删除', life: 3000 })
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '删除失败',
      detail: e instanceof ApiError ? e.message : '删除失败，请稍后再试',
      life: 4000,
    })
  }
}
</script>

<template>
  <div class="path-editor">
    <AppSurface v-if="loadError" padding="lg" class="state-surface">
      <AppErrorState :description="loadError" retry-label="重新加载" @retry="load" />
      <RouterLink :to="{ name: 'learning-paths' }" class="back-link">
        <Button label="返回学习路线列表" icon="pi pi-arrow-left" text />
      </RouterLink>
    </AppSurface>

    <AppLoadingState v-else-if="loading && isEdit && !path" label="正在加载学习路线" />

    <template v-else>
      <AppPageHeader
        :eyebrow="isEdit ? 'LEARNING // EDIT' : 'LEARNING // NEW'"
        :title="isEdit ? '编辑学习路线' : '新建学习路线'"
        description="配置路线信息与节点树。新建路线保存后即可添加节点。"
      >
        <template #actions>
          <Button label="取消" severity="secondary" text icon="pi pi-times" @click="cancel" />
          <Button
            v-if="isEdit"
            label="删除路线"
            severity="danger"
            text
            icon="pi pi-trash"
            @click="confirmDelete"
          />
          <Button
            label="保存"
            icon="pi pi-check"
            :loading="saving"
            class="save-path"
            @click="save"
          />
        </template>
      </AppPageHeader>

      <AppSurface padding="lg" class="form-surface">
        <div class="editor-grid">
          <AppFormField v-slot="{ id }" label="标题" required :hint="`${form.title.length}/100`">
            <InputText
              :id="id"
              v-model="form.title"
              class="path-title"
              maxlength="100"
              fluid
              placeholder="路线标题"
            />
          </AppFormField>

          <AppFormField v-slot="{ id }" label="封面链接">
            <InputText
              :id="id"
              v-model="form.coverUrl"
              class="path-cover"
              maxlength="512"
              fluid
              placeholder="http(s) 封面地址（可选）"
            />
          </AppFormField>

          <AppFormField v-slot="{ id }" label="排序">
            <InputNumber v-model="form.sortOrder" :input-id="id" :min="0" fluid />
          </AppFormField>

          <div class="toggle-block">
            <span class="toggle-label">选项</span>
            <div class="toggle-row">
              <span class="toggle-field">
                <ToggleSwitch
                  v-model="publicModel"
                  :disabled="forcePublic"
                  input-id="path-public"
                />
                <label for="path-public">公开</label>
              </span>
              <span v-if="showOfficialToggle" class="toggle-field">
                <ToggleSwitch v-model="form.isOfficial" input-id="path-official" />
                <label for="path-official">官方路线</label>
              </span>
            </div>
          </div>

          <AppFormField class="span-2" label="描述">
            <MarkdownEditor v-model="form.descriptionMd" :theme="isDark ? 'dark' : 'light'" />
          </AppFormField>
        </div>
      </AppSurface>

      <AppSurface v-if="isEdit && path" padding="lg" class="nodes-surface">
        <div class="section-head">
          <h2>节点</h2>
          <Button
            label="添加根节点"
            icon="pi pi-plus"
            size="small"
            severity="secondary"
            outlined
            @click="openAddRoot"
          />
        </div>

        <div class="tree-scroll">
          <TreeTable
            v-model:expanded-keys="expandedKeys"
            :value="treeNodes"
            data-key="id"
            table-style="min-width: 46rem"
            class="tree-table"
          >
            <Column field="title" header="节点" expander>
              <template #body="{ node }">
                <span class="node-title">{{ node.data.title }}</span>
                <Tag
                  :value="learningStatusLabel(node.data.status)"
                  :severity="learningStatusSeverity(node.data.status)"
                  class="node-status"
                />
              </template>
            </Column>
            <Column header="题目数" style="width: 6rem">
              <template #body="{ node }">
                <span class="count-num">{{ node.data.problemIds.length }}</span>
              </template>
            </Column>
            <Column header="完成题数" style="width: 7rem">
              <template #body="{ node }">
                <span class="count-num">{{ node.data.requiredCount }}</span>
              </template>
            </Column>
            <Column header="操作" style="min-width: 22rem">
              <template #body="{ node }">
                <div class="row-actions">
                  <Button label="添加子节点" text size="small" @click="openAddChild(node.data)" />
                  <Button label="编辑" text size="small" @click="openEdit(node.data)" />
                  <Button
                    label="上移"
                    text
                    size="small"
                    :disabled="!canMoveUp(node.data) || reordering"
                    @click="moveNode(node.data, -1)"
                  />
                  <Button
                    label="下移"
                    text
                    size="small"
                    :disabled="!canMoveDown(node.data) || reordering"
                    @click="moveNode(node.data, 1)"
                  />
                  <Button
                    label="删除"
                    text
                    size="small"
                    severity="danger"
                    :disabled="node.data.children.length > 0"
                    @click="confirmDeleteNode(node.data)"
                  />
                </div>
              </template>
            </Column>
            <template #empty>
              <AppEmptyState
                icon="pi pi-sitemap"
                title="还没有节点"
                description="点击“添加根节点”开始搭建路线。"
              />
            </template>
          </TreeTable>
        </div>
      </AppSurface>

      <AppSurface v-else-if="!isEdit" padding="lg" class="hint-surface">
        <Message severity="info" :closable="false">先保存路线，再添加节点。</Message>
      </AppSurface>
    </template>

    <LearningNodeDialog
      v-if="isEdit && path"
      v-model:visible="nodeDialogVisible"
      :path-id="pathId ?? 0"
      :nodes="nodes"
      :editing="nodeEditing"
      :parent-id="nodeParentId"
      @saved="onNodeSaved"
    />
  </div>
</template>

<style scoped>
.path-editor {
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

.form-surface,
.nodes-surface,
.hint-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
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
}

.section-head h2 {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
}

.tree-scroll {
  overflow-x: auto;
}

.tree-table {
  min-width: 0;
}

.tree-table :deep(.p-treetable-thead > tr > th) {
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

.tree-table :deep(.p-treetable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.node-title {
  margin-right: var(--kapoj-space-2);
  font-weight: 500;
}

.node-status {
  margin-left: 0;
}

.count-num {
  color: var(--kapoj-text-secondary);
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
}

@media (max-width: 800px) {
  .editor-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
