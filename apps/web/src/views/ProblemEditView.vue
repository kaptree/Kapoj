<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputChips from 'primevue/inputchips'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import SelectButton from 'primevue/selectbutton'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import {
  assessProblemDifficulty,
  createProblem,
  deleteProblem,
  getProblem,
  getProblemDifficultyHistory,
  updateProblem,
  uploadStatementImage,
} from '@/api/problems'
import type { DifficultyHistoryItem, ProblemDetail, ProblemInput, ProblemType } from '@/api/types'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import ProblemAttachmentManager from '@/components/ProblemAttachmentManager.vue'
import SampleEditor from '@/components/SampleEditor.vue'
import TestcaseManager from '@/components/TestcaseManager.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useTheme } from '@/theme/theme'
import { formatDateTime } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const { isDark } = useTheme()

const TYPE_OPTIONS: { label: string; value: ProblemType }[] = [
  { label: '传统题', value: 'traditional' },
  { label: '特判题', value: 'special' },
  { label: '交互题（暂不评测）', value: 'interactive' },
  { label: '客观题（直接提交答案）', value: 'objective' },
]

const DIFFICULTY_SOURCE_LABELS: Record<string, string> = {
  ai: 'AI 初评',
  ai_initial: 'AI 初评',
  ai_reassess: 'AI 重评',
  default: '默认',
  pass_rate: '通过率',
  manual: '手动',
  migration: '迁移',
  migrated: '迁移',
  import: '导入',
}

const HISTORY_PAGE_SIZE = 10

const problemId = computed(() =>
  typeof route.params.id === 'string' ? Number(route.params.id) : null,
)
const isEdit = computed(() => problemId.value !== null)
const canEdit = ref(true)

const pageEyebrow = computed(() => (isEdit.value ? 'PROBLEMS // EDIT' : 'PROBLEMS // CREATE'))
const pageTitle = computed(() => (isEdit.value ? '编辑题目' : '新建题目'))
const pageDescription = computed(() =>
  isEdit.value ? '编辑题目信息、题面、样例与测试点。' : '创建一道新题目，填写基本信息与题面内容。',
)

// Every field is always present in the form, unlike in the API's input type.
type FormState = Required<ProblemInput>

function emptyForm(): FormState {
  return {
    title: '',
    slug: '',
    statementMd: '',
    inputFormatMd: '',
    outputFormatMd: '',
    samples: [],
    hintMd: '',
    type: 'traditional',
    answers: [],
    tags: [],
    difficulty: 1,
    difficultyAuto: true,
    timeLimitMs: 1000,
    memoryLimitMb: 256,
    isPublic: false,
    source: '',
  }
}

const form = reactive<FormState>(emptyForm())
const answersText = ref('')
const checkerUploaded = ref(false)
const published = ref(false)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const loadFailed = ref(false)
const notFound = ref(false)

const difficultySource = ref('')
const difficultyUpdatedAt = ref('')
const difficultyReason = ref('')
const difficultySourceLabel = computed(
  () => DIFFICULTY_SOURCE_LABELS[difficultySource.value] ?? difficultySource.value,
)

const reassessing = ref(false)
const reassessWarning = ref('')

const historyOpen = ref(false)
const historyLoading = ref(false)
const historyError = ref('')
const historyItems = ref<DifficultyHistoryItem[]>([])
const historyTotal = ref(0)
const historyPage = ref(1)

function fill(p: ProblemDetail) {
  Object.assign(form, {
    title: p.title,
    slug: p.slug,
    statementMd: p.statementMd,
    inputFormatMd: p.inputFormatMd,
    outputFormatMd: p.outputFormatMd,
    samples: p.samples.map((s) => ({ ...s })),
    hintMd: p.hintMd,
    type: p.type,
    answers: [...(p.answers ?? [])],
    tags: [...p.tags],
    difficulty: p.difficulty,
    difficultyAuto: p.difficultyAuto ?? true,
    timeLimitMs: p.timeLimitMs,
    memoryLimitMb: p.memoryLimitMb,
    isPublic: p.isPublic,
    source: p.source,
  })
  difficultySource.value = p.difficultySource ?? ''
  difficultyUpdatedAt.value = p.difficultyUpdatedAt ?? ''
  difficultyReason.value = p.difficultyReason ?? ''
  reassessWarning.value = ''
  answersText.value = form.answers.join('\n')
  checkerUploaded.value = p.checkerUploaded ?? false
  published.value = p.isPublic
  canEdit.value = p.canEdit
}

function resetDifficultyMeta() {
  difficultySource.value = ''
  difficultyUpdatedAt.value = ''
  difficultyReason.value = ''
  reassessWarning.value = ''
}

async function load() {
  // The warning belongs to one problem and one reassessment; a reload may show
  // another problem, so never carry it across.
  reassessWarning.value = ''
  if (problemId.value === null) {
    Object.assign(form, emptyForm())
    resetDifficultyMeta()
    answersText.value = ''
    checkerUploaded.value = false
    published.value = false
    canEdit.value = true
    loadFailed.value = false
    notFound.value = false
    error.value = ''
    return
  }
  loading.value = true
  loadFailed.value = false
  notFound.value = false
  error.value = ''
  try {
    fill(await getProblem(problemId.value))
  } catch (e) {
    loadFailed.value = true
    if (e instanceof ApiError && e.code === 40400) {
      notFound.value = true
      error.value = '题目不存在'
    } else {
      error.value = '加载题目失败，请稍后再试'
    }
  } finally {
    loading.value = false
  }
}

watch(() => route.params.id, load, { immediate: true })

// Answers belong to objective problems only; drop them when switching type so
// a later save cannot carry stale answers into a code problem.
watch(
  () => form.type,
  (type, previous) => {
    if (type !== 'objective' && previous === 'objective') {
      answersText.value = ''
      form.answers = []
    }
  },
)

async function uploadStatementImageFile(file: File): Promise<string> {
  if (problemId.value === null) {
    throw new Error('请先保存题目，再上传图片')
  }
  const result = await uploadStatementImage(problemId.value, file)
  return result.url
}

function describe(e: unknown, fallback: string) {
  return e instanceof ApiError ? e.message : fallback
}

// Editing the score is a manual decision: lock the problem against the
// scheduled recalculation, but keep the chosen value for later saves.
function onDifficultyInput(value: number | null | undefined) {
  if (value == null) return
  reassessWarning.value = ''
  form.difficultyAuto = false
}

// Toggling the switch changes the lock state that the warning describes.
function onDifficultyAutoChange() {
  reassessWarning.value = ''
}

// Spec §4.2: tags are trimmed, empties removed, capped at 10 before submit.
function normalizedForm(): FormState {
  return {
    ...form,
    tags: form.tags
      .map((t) => t.trim())
      .filter(Boolean)
      .slice(0, 10),
    answers:
      form.type === 'objective'
        ? answersText.value
            .split(/\r?\n/)
            .map((s) => s.trim())
            .filter(Boolean)
        : [],
  }
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    if (problemId.value !== null) {
      fill(await updateProblem(problemId.value, normalizedForm()))
      reassessWarning.value = ''
      toast.add({ severity: 'success', summary: '已保存', life: 3000 })
    } else {
      const created = await createProblem(normalizedForm())
      reassessWarning.value = ''
      toast.add({
        severity: 'success',
        summary:
          form.type === 'objective'
            ? '题目已创建。请确认正确答案，再回到这里发布。'
            : '题目已创建。请上传测试点，再回到这里发布。',
        life: 3000,
      })
      await router.replace({ name: 'problem-edit', params: { id: created.id } })
    }
  } catch (e) {
    error.value = describe(e, '保存失败，请稍后再试')
    form.isPublic = published.value
  } finally {
    saving.value = false
  }
}

async function reloadFlags() {
  if (problemId.value === null) return
  try {
    const p = await getProblem(problemId.value)
    checkerUploaded.value = p.checkerUploaded ?? false
  } catch {
    // The flag only drives a label; a failed refresh is harmless.
  }
}

function remove() {
  if (problemId.value === null) return
  confirm.require({
    header: '删除题目',
    message: '确定删除这道题目吗？已有提交记录的题目只会被下线。',
    acceptLabel: '删除',
    rejectLabel: '取消',
    accept: async () => {
      if (problemId.value === null) return
      try {
        const { offlined } = await deleteProblem(problemId.value)
        toast.add({
          severity: 'success',
          summary: offlined ? '该题已有提交记录，已下线而不是删除' : '题目已删除',
          life: 3000,
        })
        await router.replace({ name: 'problems' })
      } catch (e) {
        error.value = describe(e, '删除失败，请稍后再试')
      }
    },
  })
}

function reassess() {
  if (problemId.value === null) return
  confirm.require({
    header: 'AI 重新评估',
    message: '将调用平台 AI 重新评估当前题目的难度并覆盖现有难度分，确定继续吗？',
    acceptLabel: '重新评估',
    rejectLabel: '取消',
    accept: async () => {
      if (problemId.value === null) return
      reassessing.value = true
      error.value = ''
      try {
        const updated = await assessProblemDifficulty(problemId.value)
        fill(updated)
        if (updated.difficultyAuto ?? true) {
          reassessWarning.value = ''
          toast.add({ severity: 'success', summary: 'AI 评分已更新', life: 3000 })
        } else {
          reassessWarning.value = 'AI 评分已更新，自动调整仍关闭'
          toast.add({ severity: 'warn', summary: reassessWarning.value, life: 5000 })
        }
      } catch (e) {
        error.value = describe(e, 'AI 重新评估失败，请稍后再试')
      } finally {
        reassessing.value = false
      }
    },
  })
}

async function loadHistory() {
  if (problemId.value === null) return
  historyLoading.value = true
  historyError.value = ''
  try {
    const result = await getProblemDifficultyHistory(
      problemId.value,
      historyPage.value,
      HISTORY_PAGE_SIZE,
    )
    historyItems.value = result.items
    historyTotal.value = result.total
  } catch (e) {
    historyError.value = describe(e, '加载难度历史失败，请稍后再试')
  } finally {
    historyLoading.value = false
  }
}

function openHistory() {
  if (problemId.value === null) return
  historyPage.value = 1
  historyItems.value = []
  historyTotal.value = 0
  historyOpen.value = true
  void loadHistory()
}

function onHistoryPage(event: { page: number }) {
  historyPage.value = event.page + 1
  void loadHistory()
}

function formatPercent(value: number) {
  return `${(value * 100).toFixed(1)}%`
}
</script>

<template>
  <div class="page">
    <AppPageHeader :eyebrow="pageEyebrow" :title="pageTitle" :description="pageDescription">
      <template #actions>
        <Button
          as="router-link"
          :to="{ name: 'problems' }"
          label="返回题库"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
      </template>
    </AppPageHeader>

    <AppSurface v-if="loading" padding="lg">
      <AppLoadingState label="正在加载题目" />
    </AppSurface>

    <AppSurface v-else-if="loadFailed && notFound" padding="lg">
      <AppEmptyState
        icon="pi pi-search"
        title="题目不存在"
        description="这道题目可能已被删除，或你没有访问权限。"
      >
        <template #actions>
          <Button as="router-link" :to="{ name: 'problems' }" label="返回题库" />
        </template>
      </AppEmptyState>
    </AppSurface>

    <AppSurface v-else-if="loadFailed" padding="lg">
      <AppErrorState :description="error" @retry="load" />
      <div class="error-actions">
        <Button
          as="router-link"
          :to="{ name: 'problems' }"
          label="返回题库"
          severity="secondary"
          outlined
        />
      </div>
    </AppSurface>

    <AppSurface v-else-if="!canEdit" padding="lg">
      <AppEmptyState
        icon="pi pi-lock"
        title="无权编辑此题目"
        description="出题人只能编辑自己创建的题目；如需修改，请联系题目管理员。"
      >
        <template #actions>
          <Button as="router-link" :to="{ name: 'problems' }" label="返回题库" />
        </template>
      </AppEmptyState>
    </AppSurface>

    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <Message
        v-if="reassessWarning"
        severity="warn"
        :closable="false"
        data-test="difficulty-reassess-warning"
      >
        {{ reassessWarning }}
      </Message>
      <Message v-if="!isEdit" severity="info" :closable="false">
        {{
          form.type === 'objective'
            ? '新题目创建后默认不公开。填写正确答案后回到这里发布。'
            : '新题目创建后默认不公开。创建后上传测试点，再回到这里发布。'
        }}
      </Message>

      <AppSurface padding="lg">
        <form class="form" @submit.prevent="save">
          <div class="grid">
            <AppFormField label="标题" required>
              <template #default="{ id }">
                <InputText :id="id" v-model="form.title" name="title" maxlength="100" />
              </template>
            </AppFormField>

            <AppFormField label="题目标识" required>
              <template #default="{ id }">
                <InputText
                  :id="id"
                  v-model="form.slug"
                  name="slug"
                  placeholder="小写字母、数字和连字符，例如 a-plus-b"
                />
              </template>
            </AppFormField>

            <AppFormField label="类型">
              <template #default="{ id }">
                <SelectButton
                  v-model="form.type"
                  :options="TYPE_OPTIONS"
                  option-label="label"
                  option-value="value"
                  :allow-empty="false"
                  :aria-labelledby="id"
                />
              </template>
            </AppFormField>

            <AppFormField
              v-if="form.type === 'objective'"
              class="grid-full"
              label="正确答案"
              required
              hint="每行一个可接受答案；判定时忽略首尾空白和大小写"
            >
              <template #default="{ id }">
                <Textarea
                  :id="id"
                  v-model="answersText"
                  data-test="objective-answers"
                  rows="6"
                  fluid
                />
              </template>
            </AppFormField>

            <AppFormField class="grid-full" label="难度">
              <template #default="{ id }">
                <div class="difficulty">
                  <div class="difficulty-row">
                    <InputNumber
                      v-model="form.difficulty"
                      data-test="difficulty-input"
                      :input-id="id"
                      :min="1"
                      :max="100"
                      :step="1"
                      :disabled="form.difficultyAuto"
                      @update:model-value="onDifficultyInput"
                    />
                    <Tag
                      v-if="isEdit && difficultySource"
                      data-test="difficulty-source"
                      :value="difficultySourceLabel"
                      severity="secondary"
                    />
                    <div class="difficulty-auto">
                      <ToggleSwitch
                        v-model="form.difficultyAuto"
                        data-test="difficulty-auto"
                        input-id="difficulty-auto"
                        @update:model-value="onDifficultyAutoChange"
                      />
                      <label for="difficulty-auto">允许系统自动调整</label>
                    </div>
                  </div>
                  <p class="difficulty-meta">
                    <span>{{
                      form.difficultyAuto
                        ? '每天按全站通过率自动调整难度。'
                        : '已锁定当前难度，定时任务不会覆盖。'
                    }}</span>
                    <span v-if="isEdit && difficultyUpdatedAt" class="difficulty-updated">
                      最近更新：{{ formatDateTime(difficultyUpdatedAt) }}
                    </span>
                    <span v-if="isEdit && difficultyReason" class="difficulty-reason">
                      理由：{{ difficultyReason }}
                    </span>
                  </p>
                  <div v-if="isEdit" class="difficulty-actions">
                    <Button
                      type="button"
                      label="AI 重新评估"
                      icon="pi pi-sparkles"
                      severity="secondary"
                      outlined
                      :loading="reassessing"
                      @click="reassess"
                    />
                    <Button
                      type="button"
                      label="查看难度历史"
                      icon="pi pi-history"
                      severity="secondary"
                      text
                      @click="openHistory"
                    />
                  </div>
                </div>
              </template>
            </AppFormField>

            <AppFormField v-if="form.type !== 'objective'" label="时间限制">
              <template #default="{ id }">
                <div class="unit-row">
                  <InputNumber
                    v-model="form.timeLimitMs"
                    :input-id="id"
                    :min="100"
                    :max="10000"
                    :step="100"
                  />
                  <span class="unit">毫秒</span>
                </div>
              </template>
            </AppFormField>

            <AppFormField v-if="form.type !== 'objective'" label="内存限制">
              <template #default="{ id }">
                <div class="unit-row">
                  <InputNumber
                    v-model="form.memoryLimitMb"
                    :input-id="id"
                    :min="16"
                    :max="1024"
                    :step="16"
                  />
                  <span class="unit">MB</span>
                </div>
              </template>
            </AppFormField>

            <AppFormField label="标签">
              <template #default="{ id }">
                <InputChips
                  v-model="form.tags"
                  :input-id="id"
                  :max="10"
                  :allow-duplicate="false"
                  add-on-blur
                  placeholder="输入后回车添加，最多 10 个"
                />
              </template>
            </AppFormField>

            <AppFormField label="来源">
              <template #default="{ id }">
                <InputText :id="id" v-model="form.source" name="source" maxlength="200" />
              </template>
            </AppFormField>
          </div>

          <Tabs value="description" lazy>
            <TabList>
              <Tab value="description">题目描述</Tab>
              <Tab value="input">输入格式</Tab>
              <Tab value="output">输出格式</Tab>
              <Tab value="samples">样例</Tab>
              <Tab value="hint">提示</Tab>
            </TabList>
            <TabPanels>
              <TabPanel value="description">
                <MarkdownEditor
                  v-model="form.statementMd"
                  :theme="isDark ? 'dark' : 'light'"
                  :problem-id="problemId ?? undefined"
                  :upload-image="uploadStatementImageFile"
                />
              </TabPanel>
              <TabPanel value="input">
                <MarkdownEditor
                  v-model="form.inputFormatMd"
                  height="240px"
                  :theme="isDark ? 'dark' : 'light'"
                  :problem-id="problemId ?? undefined"
                  :upload-image="uploadStatementImageFile"
                />
              </TabPanel>
              <TabPanel value="output">
                <MarkdownEditor
                  v-model="form.outputFormatMd"
                  height="240px"
                  :theme="isDark ? 'dark' : 'light'"
                  :problem-id="problemId ?? undefined"
                  :upload-image="uploadStatementImageFile"
                />
              </TabPanel>
              <TabPanel value="samples">
                <SampleEditor v-model="form.samples" />
              </TabPanel>
              <TabPanel value="hint">
                <MarkdownEditor
                  v-model="form.hintMd"
                  height="240px"
                  :theme="isDark ? 'dark' : 'light'"
                  :problem-id="problemId ?? undefined"
                  :upload-image="uploadStatementImageFile"
                />
              </TabPanel>
            </TabPanels>
          </Tabs>

          <AppFormField label="公开">
            <template #default="{ id }">
              <div class="public-row">
                <ToggleSwitch
                  v-model="form.isPublic"
                  data-test="public-toggle"
                  :input-id="id"
                  :disabled="!isEdit"
                />
                <span class="unit">{{
                  form.type === 'objective'
                    ? '客观题无需测试点；发布前请确认至少 1 个正确答案。'
                    : '发布需要至少 1 个测试点；特判题还需要 checker。'
                }}</span>
              </div>
            </template>
          </AppFormField>

          <div class="actions">
            <Button
              type="submit"
              :label="isEdit ? '保存修改' : '创建题目'"
              icon="pi pi-check"
              :loading="saving"
            />
            <Button
              as="router-link"
              :to="{ name: 'problems' }"
              label="返回题库"
              severity="secondary"
              outlined
            />
            <Button v-if="isEdit" label="删除题目" severity="danger" outlined @click="remove" />
          </div>
        </form>
      </AppSurface>

      <AppSurface v-if="isEdit && form.type !== 'objective'" padding="lg" class="testcases">
        <TestcaseManager
          :problem-id="problemId!"
          :type="form.type"
          :checker-uploaded="checkerUploaded"
          @changed="reloadFlags"
        />
      </AppSurface>

      <AppSurface v-if="isEdit" padding="lg" class="attachments">
        <ProblemAttachmentManager :problem-id="problemId!" />
      </AppSurface>

      <Dialog
        v-model:visible="historyOpen"
        header="难度历史"
        modal
        :style="{ width: 'min(60rem, 94vw)' }"
      >
        <AppErrorState v-if="historyError" :description="historyError" @retry="loadHistory" />
        <AppLoadingState
          v-else-if="historyLoading && historyItems.length === 0"
          label="正在加载难度历史"
        />
        <DataTable
          v-else
          :value="historyItems"
          data-key="createdAt"
          size="small"
          responsive-layout="scroll"
          data-test="difficulty-history-table"
        >
          <Column header="时间" style="width: 170px">
            <template #body="{ data }">
              <span class="history-muted">{{ formatDateTime(data.createdAt) }}</span>
            </template>
          </Column>
          <Column header="变化" style="width: 110px">
            <template #body="{ data }">
              <span class="history-score">{{ data.oldScore }} → {{ data.newScore }}</span>
            </template>
          </Column>
          <Column header="来源" style="width: 110px">
            <template #body="{ data }">
              {{ DIFFICULTY_SOURCE_LABELS[data.source] ?? data.source }}
            </template>
          </Column>
          <Column header="平滑通过率" style="width: 120px">
            <template #body="{ data }">{{ formatPercent(data.smoothedPassRate) }}</template>
          </Column>
          <Column header="理由" style="min-width: 180px">
            <template #body="{ data }">{{ data.reason || '—' }}</template>
          </Column>
          <template #empty>
            <AppEmptyState
              icon="pi pi-history"
              title="暂无难度历史"
              description="难度变更后会在这里显示。"
            />
          </template>
        </DataTable>
        <Paginator
          v-if="historyTotal > HISTORY_PAGE_SIZE"
          :rows="HISTORY_PAGE_SIZE"
          :total-records="historyTotal"
          :first="(historyPage - 1) * HISTORY_PAGE_SIZE"
          :page-link-size="5"
          template="PrevPageLink PageLinks NextPageLink"
          class="history-pager"
          @page="onHistoryPage"
        />
      </Dialog>
    </template>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-4);
}

.grid-full {
  grid-column: 1 / -1;
}

.unit-row,
.public-row {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.unit {
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.difficulty {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
}

.difficulty-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
}

.difficulty-auto {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.difficulty-auto label {
  cursor: pointer;
  user-select: none;
}

.difficulty-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
  line-height: 1.5;
}

.difficulty-meta .difficulty-updated,
.difficulty-meta .difficulty-reason {
  padding-left: var(--kapoj-space-2);
  border-left: 1px solid var(--kapoj-border);
}

.difficulty-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.history-muted {
  color: var(--kapoj-text-secondary);
}

.history-score {
  font-family: var(--kapoj-font-mono);
  font-weight: 600;
}

.history-pager {
  margin-top: var(--kapoj-space-3);
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.testcases {
  display: flex;
  flex-direction: column;
}

.error-actions {
  margin-top: var(--kapoj-space-3);
}

@media (max-width: 640px) {
  .grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
