<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputChips from 'primevue/inputchips'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import { createProblem, deleteProblem, getProblem, updateProblem } from '@/api/problems'
import type { ProblemDetail, ProblemInput, ProblemType } from '@/api/types'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import SampleEditor from '@/components/SampleEditor.vue'
import TestcaseManager from '@/components/TestcaseManager.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useTheme } from '@/theme/theme'
import { DIFFICULTIES } from '@/utils/difficulty'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const { isDark } = useTheme()

const TYPE_OPTIONS: { label: string; value: ProblemType }[] = [
  { label: '传统题', value: 'traditional' },
  { label: '特判题', value: 'special' },
  { label: '交互题（暂不评测）', value: 'interactive' },
]

const problemId = computed(() =>
  typeof route.params.id === 'string' ? Number(route.params.id) : null,
)
const isEdit = computed(() => problemId.value !== null)

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
    tags: [],
    difficulty: 1,
    timeLimitMs: 1000,
    memoryLimitMb: 256,
    isPublic: false,
    source: '',
  }
}

const form = reactive<FormState>(emptyForm())
const checkerUploaded = ref(false)
const published = ref(false)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const loadFailed = ref(false)
const notFound = ref(false)

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
    tags: [...p.tags],
    difficulty: p.difficulty,
    timeLimitMs: p.timeLimitMs,
    memoryLimitMb: p.memoryLimitMb,
    isPublic: p.isPublic,
    source: p.source,
  })
  checkerUploaded.value = p.checkerUploaded ?? false
  published.value = p.isPublic
}

async function load() {
  if (problemId.value === null) {
    Object.assign(form, emptyForm())
    checkerUploaded.value = false
    published.value = false
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

function describe(e: unknown, fallback: string) {
  return e instanceof ApiError ? e.message : fallback
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    if (problemId.value !== null) {
      fill(await updateProblem(problemId.value, form))
      toast.add({ severity: 'success', summary: '已保存', life: 3000 })
    } else {
      const created = await createProblem(form)
      toast.add({
        severity: 'success',
        summary: '题目已创建。请上传测试点，再回到这里发布。',
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

    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <Message v-if="!isEdit" severity="info" :closable="false">
        新题目创建后默认不公开。创建后上传测试点，再回到这里发布。
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
              <template #default>
                <SelectButton
                  v-model="form.type"
                  :options="TYPE_OPTIONS"
                  option-label="label"
                  option-value="value"
                  :allow-empty="false"
                />
              </template>
            </AppFormField>

            <AppFormField label="难度">
              <template #default="{ id }">
                <Select
                  v-model="form.difficulty"
                  :input-id="id"
                  :options="DIFFICULTIES"
                  option-label="label"
                  option-value="value"
                />
              </template>
            </AppFormField>

            <AppFormField label="时间限制">
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

            <AppFormField label="内存限制">
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
                <MarkdownEditor v-model="form.statementMd" :theme="isDark ? 'dark' : 'light'" />
              </TabPanel>
              <TabPanel value="input">
                <MarkdownEditor
                  v-model="form.inputFormatMd"
                  height="240px"
                  :theme="isDark ? 'dark' : 'light'"
                />
              </TabPanel>
              <TabPanel value="output">
                <MarkdownEditor
                  v-model="form.outputFormatMd"
                  height="240px"
                  :theme="isDark ? 'dark' : 'light'"
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
                />
              </TabPanel>
            </TabPanels>
          </Tabs>

          <AppFormField label="公开">
            <template #default="{ id }">
              <div class="public-row">
                <ToggleSwitch v-model="form.isPublic" :input-id="id" :disabled="!isEdit" />
                <span class="unit">发布需要至少 1 个测试点；特判题还需要 checker。</span>
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

      <AppSurface v-if="isEdit" padding="lg" class="testcases">
        <TestcaseManager
          :problem-id="problemId!"
          :type="form.type"
          :checker-uploaded="checkerUploaded"
          @changed="reloadFlags"
        />
      </AppSurface>
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
