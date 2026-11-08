<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import {
  confirmTestcaseGeneration,
  createTestcaseGeneration,
  discardTestcaseGeneration,
  getTestcaseGeneration,
  previewTestcaseGeneration,
} from '@/api/problems'
import type {
  SubmissionLanguage,
  TestcaseGenerationPreview,
  TestcaseGenerationStatus,
  TestcaseGenerationView,
} from '@/api/types'
import CodeEditor from '@/components/CodeEditor.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatBytes } from '@/utils/format'
import { monacoLanguage, SUBMISSION_LANGUAGES } from '@/utils/submission'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

const problemId = computed(() => Number(route.params.id))

const language = ref<SubmissionLanguage>('cpp')
const sourceCode = ref('')
const count = ref<number | null>(5)

const creating = ref(false)
const confirming = ref(false)
const discarding = ref(false)
const generation = ref<TestcaseGenerationView | null>(null)
const error = ref('')
const showAiConfigLink = ref(false)
const timedOut = ref(false)

const STATUS_LABEL: Record<TestcaseGenerationStatus, string> = {
  pending: '排队中',
  running: '生成中',
  ready: '待确认',
  failed: '生成失败',
  confirmed: '已入库',
  discarded: '已丢弃',
}

const STATUS_SEVERITY: Record<
  TestcaseGenerationStatus,
  'secondary' | 'success' | 'info' | 'warn' | 'danger'
> = {
  pending: 'secondary',
  running: 'warn',
  ready: 'info',
  failed: 'danger',
  confirmed: 'success',
  discarded: 'secondary',
}

const MAX_SOURCE_BYTES = 65536

const cases = computed(() => generation.value?.cases ?? [])
const status = computed<TestcaseGenerationStatus | null>(() => generation.value?.status ?? null)
const statusLabel = computed(() => (status.value ? STATUS_LABEL[status.value] : ''))
const statusSeverity = computed<'secondary' | 'success' | 'info' | 'warn' | 'danger'>(() =>
  status.value ? STATUS_SEVERITY[status.value] : 'secondary',
)
const isActive = computed(
  () => !timedOut.value && (status.value === 'pending' || status.value === 'running'),
)

// Preview dialog state.
const previewVisible = ref(false)
const previewLoading = ref(false)
const preview = ref<TestcaseGenerationPreview | null>(null)
const previewKind = ref<'input' | 'output'>('input')

function byteLength(value: string): number {
  return new TextEncoder().encode(value).length
}

/** Substrings that mark a 40001 as a missing user AI configuration (worth a settings link). */
const AI_CONFIG_KEYWORDS = ['AI 模型', 'AI 设置', 'API Key', '服务商']

function isAIConfigMissing(message: string): boolean {
  return AI_CONFIG_KEYWORDS.some((k) => message.includes(k))
}

/** Maps an API error to a user-facing message and whether to offer the AI settings link. */
function mapError(e: unknown): { message: string; aiLink: boolean } {
  if (e instanceof ApiError) {
    if (e.code === 40002) return { message: '管理员尚未配置 AI 加密密钥', aiLink: false }
    if (e.code === 40001) {
      const msg = e.message || '操作失败，请稍后再试'
      return { message: msg, aiLink: isAIConfigMissing(msg) }
    }
    if (e.code === 42903 || e.code === 50200 || e.code === 50300) {
      return { message: e.message || 'AI 服务暂时不可用，请稍后再试', aiLink: false }
    }
    if (e.status === 404) return { message: '题目不存在或没有权限', aiLink: false }
    return { message: e.message || '操作失败，请稍后再试', aiLink: false }
  }
  return { message: '操作失败，请稍后再试', aiLink: false }
}

function applyCreateError(e: unknown) {
  const mapped = mapError(e)
  error.value = mapped.message
  showAiConfigLink.value = mapped.aiLink
  toast.add({ severity: 'error', summary: '生成失败', detail: mapped.message, life: 4000 })
}

async function create() {
  const src = sourceCode.value
  if (!src.trim()) {
    toast.add({ severity: 'warn', summary: '源码不能为空', life: 3000 })
    return
  }
  if (byteLength(src) > MAX_SOURCE_BYTES) {
    toast.add({ severity: 'warn', summary: '源码超过 64 KiB', life: 3000 })
    return
  }
  const c = count.value
  if (c === null || !Number.isInteger(c) || c < 1 || c > 10) {
    toast.add({ severity: 'warn', summary: '生成组数必须是 1 到 10 之间的整数', life: 3000 })
    return
  }

  creating.value = true
  error.value = ''
  showAiConfigLink.value = false
  timedOut.value = false
  try {
    const gen = await createTestcaseGeneration(problemId.value, {
      language: language.value,
      sourceCode: src,
      testcaseCount: c,
    })
    generation.value = gen
    if (!isTerminal(gen.status)) startPolling()
  } catch (e) {
    applyCreateError(e)
  } finally {
    creating.value = false
  }
}

const POLL_INTERVAL_MS = 2000
const POLL_TIMEOUT_MS = 10 * 60 * 1000

let pollTimer: number | undefined
let pollDeadline = 0
let cancelled = false

function isTerminal(s: TestcaseGenerationStatus): boolean {
  return s === 'ready' || s === 'failed' || s === 'confirmed' || s === 'discarded'
}

/** A poll GET failure that will not recover by retrying (e.g. generation gone). */
function isTerminalPollError(e: unknown): boolean {
  return e instanceof ApiError && (e.status === 404 || e.status === 403)
}

function stopPolling() {
  cancelled = true
  if (pollTimer !== undefined) {
    window.clearTimeout(pollTimer)
    pollTimer = undefined
  }
}

function startPolling() {
  stopPolling()
  cancelled = false
  pollDeadline = Date.now() + POLL_TIMEOUT_MS
  schedulePoll()
}

function schedulePoll() {
  if (cancelled) return
  pollTimer = window.setTimeout(async () => {
    pollTimer = undefined
    if (cancelled) return
    const gen = generation.value
    if (!gen) return
    if (Date.now() >= pollDeadline) {
      timedOut.value = true
      return
    }
    try {
      const next = await getTestcaseGeneration(problemId.value, gen.id)
      if (cancelled) return
      generation.value = next
      if (!isTerminal(next.status)) schedulePoll()
    } catch (e) {
      if (cancelled) return
      if (isTerminalPollError(e)) {
        const mapped = mapError(e)
        error.value = mapped.message
        showAiConfigLink.value = mapped.aiLink
        toast.add({
          severity: 'error',
          summary: '查询生成状态失败',
          detail: mapped.message,
          life: 4000,
        })
        return
      }
      // A transient GET failure should not abort the poll; keep going until the deadline.
      schedulePoll()
    }
  }, POLL_INTERVAL_MS)
}

onBeforeUnmount(stopPolling)

async function doConfirm(gen: TestcaseGenerationView) {
  confirming.value = true
  try {
    const result = await confirmTestcaseGeneration(problemId.value, gen.id)
    toast.add({
      severity: 'success',
      summary: `已入库 ${result.count} 组测试点`,
      life: 3000,
    })
    await router.push({ name: 'problem-edit', params: { id: problemId.value } })
  } catch (e) {
    const mapped = mapError(e)
    toast.add({ severity: 'error', summary: '确认失败', detail: mapped.message, life: 4000 })
  } finally {
    confirming.value = false
  }
}

function onConfirm() {
  const gen = generation.value
  if (!gen) return
  const n = gen.cases?.length ?? gen.testcaseCount
  confirm.require({
    header: '确认入库',
    message: `将把生成的 ${n} 组测试点写入题目，确定吗？`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '确认入库',
    rejectLabel: '取消',
    accept: () => void doConfirm(gen),
  })
}

async function doDiscard() {
  const gen = generation.value
  if (!gen) return
  discarding.value = true
  try {
    await discardTestcaseGeneration(problemId.value, gen.id)
    generation.value = null
    error.value = ''
    showAiConfigLink.value = false
    timedOut.value = false
    toast.add({ severity: 'success', summary: '已丢弃生成结果', life: 3000 })
  } catch (e) {
    const mapped = mapError(e)
    toast.add({ severity: 'error', summary: '丢弃失败', detail: mapped.message, life: 4000 })
  } finally {
    discarding.value = false
  }
}

function onDiscard() {
  confirm.require({
    header: '丢弃生成结果',
    message: '将删除已生成的输入输出并丢弃本次结果，确定吗？',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '丢弃',
    rejectLabel: '取消',
    accept: () => void doDiscard(),
  })
}

async function openPreview(caseId: number, kind: 'input' | 'output') {
  const gen = generation.value
  if (!gen) return
  previewKind.value = kind
  preview.value = null
  previewLoading.value = true
  previewVisible.value = true
  try {
    preview.value = await previewTestcaseGeneration(problemId.value, gen.id, caseId, kind)
  } catch (e) {
    previewVisible.value = false
    const mapped = mapError(e)
    toast.add({ severity: 'error', summary: '预览失败', detail: mapped.message, life: 4000 })
  } finally {
    previewLoading.value = false
  }
}

const previewTitle = computed(() => (previewKind.value === 'input' ? '输入预览' : '输出预览'))
</script>

<template>
  <div class="testgen-page">
    <AppPageHeader
      eyebrow="TESTDATA // AI"
      title="AI 造数据"
      description="提交标程，AI 生成纯 Python 3 造数据脚本并在沙盒中运行生成输入、运行标程生成输出；仅传统题支持，确认入库前不会改动题目测试数据。"
    >
      <template #actions>
        <Button
          as="router-link"
          :to="{ name: 'problem-edit', params: { id: problemId } }"
          label="返回编辑"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="testgen-surface">
      <form class="testgen-form" novalidate @submit.prevent="create">
        <AppFormField v-slot="{ id }" label="标程语言" required>
          <Select
            v-model="language"
            :input-id="id"
            :options="SUBMISSION_LANGUAGES"
            option-label="label"
            option-value="value"
            fluid
          />
        </AppFormField>

        <AppFormField label="标程源码" required hint="按 UTF-8 字节数不超过 64 KiB">
          <div class="testgen-editor">
            <CodeEditor v-model="sourceCode" :language="monacoLanguage(language)" height="320px" />
          </div>
        </AppFormField>

        <AppFormField
          v-slot="{ id }"
          label="生成组数"
          hint="1–10 组，每组生成一个输入并由标程运行得到输出"
        >
          <InputNumber v-model="count" :input-id="id" :min="1" :max="10" :show-buttons="true" />
        </AppFormField>

        <div class="testgen-actions">
          <Button
            type="submit"
            label="生成数据"
            icon="pi pi-sparkles"
            :loading="creating"
            :disabled="creating || isActive"
          />
        </div>
      </form>

      <Message v-if="error" severity="error" :closable="false" role="alert">
        {{ error }}
        <RouterLink v-if="showAiConfigLink" to="/settings/ai" class="testgen-ai-link"
          >去配置</RouterLink
        >
      </Message>
    </AppSurface>

    <AppSurface v-if="generation" padding="lg" class="testgen-surface">
      <div class="status-row">
        <span class="status-row__label">生成状态</span>
        <Tag :value="statusLabel" :severity="statusSeverity" />
      </div>

      <Message v-if="timedOut" severity="warn" :closable="false" role="status">
        生成超时，请稍后重试或重新生成
      </Message>

      <Message
        v-if="generation.status === 'failed' && generation.errorMessage"
        severity="error"
        :closable="false"
      >
        {{ generation.errorMessage }}
      </Message>

      <template v-if="generation.status === 'ready'">
        <DataTable
          :value="cases"
          data-key="id"
          size="small"
          responsive-layout="scroll"
          class="testgen-table"
        >
          <Column header="序号" style="width: 80px">
            <template #body="{ data }">{{ data.sortOrder }}</template>
          </Column>
          <Column header="输入大小" style="min-width: 120px">
            <template #body="{ data }">{{ formatBytes(data.inputSize) }}</template>
          </Column>
          <Column header="输出大小" style="min-width: 120px">
            <template #body="{ data }">{{ formatBytes(data.outputSize) }}</template>
          </Column>
          <Column header="预览" style="width: 150px">
            <template #body="{ data }">
              <div class="testgen-preview-actions">
                <Button label="输入" size="small" text @click="openPreview(data.id, 'input')" />
                <Button label="输出" size="small" text @click="openPreview(data.id, 'output')" />
              </div>
            </template>
          </Column>
        </DataTable>

        <details v-if="generation.generatorCode" class="testgen-script">
          <summary>生成脚本（只读）</summary>
          <pre>{{ generation.generatorCode }}</pre>
        </details>

        <div class="testgen-result-actions">
          <Button label="确认入库" icon="pi pi-check" :loading="confirming" @click="onConfirm" />
          <Button
            label="丢弃"
            icon="pi pi-trash"
            severity="danger"
            outlined
            :loading="discarding"
            @click="onDiscard"
          />
        </div>
      </template>

      <div v-if="generation.status === 'failed'" class="testgen-result-actions">
        <Button
          label="丢弃"
          icon="pi pi-trash"
          severity="danger"
          outlined
          :loading="discarding"
          @click="onDiscard"
        />
      </div>
    </AppSurface>

    <Dialog
      v-model:visible="previewVisible"
      :header="previewTitle"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 720px)' }"
    >
      <p v-if="previewLoading" class="testgen-preview-loading">正在加载预览…</p>
      <template v-else-if="preview">
        <pre class="testgen-preview-content">{{ preview.content }}</pre>
        <p v-if="preview.truncated" class="testgen-preview-note">仅显示前 64 KiB</p>
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.testgen-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.testgen-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.testgen-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.testgen-editor {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.testgen-actions {
  display: flex;
  gap: var(--kapoj-space-2);
}

.status-row {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 26%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-md);
}

.status-row__label {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.testgen-ai-link {
  margin-left: var(--kapoj-space-2);
  color: var(--kapoj-info);
  font-weight: 600;
  text-decoration: none;
}

.testgen-ai-link:hover {
  text-decoration: underline;
}

.testgen-table {
  min-width: 0;
}

.testgen-preview-actions {
  display: flex;
  gap: var(--kapoj-space-1);
}

.testgen-script {
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
  padding: var(--kapoj-space-3);
}

.testgen-script summary {
  cursor: pointer;
  color: var(--kapoj-text-secondary);
  font-weight: 600;
}

.testgen-script pre {
  margin: var(--kapoj-space-3) 0 0;
  padding: var(--kapoj-space-3);
  overflow: auto;
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre;
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.testgen-result-actions {
  display: flex;
  gap: var(--kapoj-space-2);
}

.testgen-preview-loading {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 13px;
}

.testgen-preview-content {
  margin: 0;
  padding: var(--kapoj-space-3);
  overflow: auto;
  font-family: var(--kapoj-font-mono);
  font-size: 13px;
  line-height: 1.5;
  white-space: pre;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.testgen-preview-note {
  margin: var(--kapoj-space-2) 0 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

@media (max-width: 640px) {
  .testgen-actions :deep(.p-button),
  .testgen-result-actions :deep(.p-button) {
    width: 100%;
  }
}
</style>
