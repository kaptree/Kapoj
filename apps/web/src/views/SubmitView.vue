<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import { useToast } from 'primevue/usetoast'

import { completeCode, formatCode, getAIConfig, optimizeCode } from '@/api/ai'
import { getContest } from '@/api/contests'
import { ApiError } from '@/api/http'
import { getProblem } from '@/api/problems'
import { createSubmission } from '@/api/submissions'
import type { AIConfigView, ContestDetail, ProblemDetail } from '@/api/types'
import CodeEditor from '@/components/CodeEditor.vue'
import MarkdownView from '@/components/MarkdownView.vue'
import AppDifficultyTag from '@/components/ui/AppDifficultyTag.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/theme/theme'
import {
  editorLanguage,
  languageLabel,
  SUBMISSION_LANGUAGES,
  type CodeSubmissionLanguage,
} from '@/utils/submission'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const auth = useAuthStore()
const { isDark } = useTheme()

const problemId = computed(() => Number(route.params.id))
const contestId = computed(() => {
  const raw = Array.isArray(route.query.contestId)
    ? route.query.contestId[0]
    : route.query.contestId
  const n = Number(raw)
  return Number.isInteger(n) && n > 0 ? n : 0
})

// Draft keys must stay stable during unmount, when route params may already be
// cleared. The submit route does not reuse the component across problems.
const draftProblemId = problemId.value
const draftContestId = contestId.value

const problem = ref<ProblemDetail | null>(null)
const contest = ref<ContestDetail | null>(null)
const contestTitle = computed(() => contest.value?.title ?? '')
const loading = ref(false)
const submitting = ref(false)
const error = ref('')
const isObjective = computed(() => problem.value?.type === 'objective')
const answer = ref('')
const language = ref<CodeSubmissionLanguage>('cpp')
const drafts = reactive<Record<CodeSubmissionLanguage, string>>({
  c: '',
  cpp: '',
  python3: '',
  java: '',
  go: '',
})

const templates: Record<CodeSubmissionLanguage, string> = {
  c: '#include <stdio.h>\n\nint main() {\n    // 在此编写代码\n    return 0;\n}\n',
  cpp: '#include <iostream>\n\nint main() {\n    // 在此编写代码\n    return 0;\n}\n',
  python3: '# 在此编写代码\n',
  java: 'public class Main {\n    public static void main(String[] args) {\n        // 在此编写代码\n    }\n}\n',
  go: 'package main\n\nimport "fmt"\n\nfunc main() {\n    // 在此编写代码\n    _ = fmt.Print\n}\n',
}

const typeLabels = {
  traditional: '传统题',
  special: '特判题',
  interactive: '交互题',
  objective: '客观题',
} as const

const code = computed({
  get: () => drafts[language.value],
  set: (value: string) => {
    drafts[language.value] = value
  },
})

const MAX_CODE_BYTES = 65536

const editorRef = ref<InstanceType<typeof CodeEditor>>()

const aiConfig = ref<AIConfigView | null>(null)
const aiAvailable = ref(true)
const aiLoadFailed = ref(false)
const aiBusy = ref<'complete' | 'format' | 'optimize' | null>(null)

const formatDialogVisible = ref(false)
const formatPreview = ref('')
const optimizeDialogVisible = ref(false)
const optimizePreview = ref('')
const optimizeSuggestions = ref('')

const aiConfigured = computed(() => aiConfig.value?.configured === true)
const aiKeyConfigured = computed(() => aiConfig.value?.keyConfigured === true)
const contestAIDisabled = computed(
  () =>
    contest.value?.status === 'running' &&
    contest.value.disableAi &&
    !auth.can(PERM.contest.update),
)
const aiButtonsDisabled = computed(
  () =>
    aiBusy.value !== null ||
    !aiAvailable.value ||
    aiLoadFailed.value ||
    !aiKeyConfigured.value ||
    !aiConfigured.value ||
    contestAIDisabled.value,
)
const showKeyMissingBanner = computed(
  () =>
    aiAvailable.value &&
    !aiLoadFailed.value &&
    aiConfig.value !== null &&
    aiConfig.value.keyConfigured === false,
)
const showConfigBanner = computed(
  () =>
    aiAvailable.value &&
    !aiLoadFailed.value &&
    aiConfig.value !== null &&
    aiConfig.value.keyConfigured !== false &&
    !aiConfig.value.configured,
)
const optimizeCanApply = computed(
  () => optimizePreview.value !== '' && optimizePreview.value !== code.value,
)

function byteLength(value: string): number {
  return new TextEncoder().encode(value).length
}

function handleAIError(e: unknown) {
  if (e instanceof ApiError) {
    if (e.code === 40302) {
      toast.add({ severity: 'warn', summary: '比赛期间已禁用 AI', life: 4000 })
      return
    }
    if (e.code === 40002) {
      aiConfig.value = {
        configured: false,
        keyConfigured: false,
        provider: '',
        modelName: '',
        apiKeyMasked: '',
        providers: aiConfig.value?.providers ?? [],
      }
      toast.add({ severity: 'warn', summary: '管理员尚未配置 AI 密钥', life: 4000 })
      return
    }
    if (e.code === 40001) {
      aiConfig.value = {
        configured: false,
        keyConfigured: true,
        provider: '',
        modelName: '',
        apiKeyMasked: '',
        providers: aiConfig.value?.providers ?? [],
      }
      toast.add({
        severity: 'warn',
        summary: '尚未配置 AI 模型',
        detail: '请先在 AI 设置中配置后使用',
        life: 4000,
      })
      return
    }
    if (e.code === 42903) {
      toast.add({ severity: 'warn', summary: '操作过于频繁，请稍后再试', life: 4000 })
      return
    }
    if (e.code === 50200 || e.code === 50300) {
      toast.add({ severity: 'error', summary: 'AI 服务暂时不可用', life: 4000 })
      return
    }
    if (e.status === 404) {
      toast.add({ severity: 'warn', summary: 'AI 功能未启用', life: 4000 })
      return
    }
  }
  toast.add({ severity: 'error', summary: 'AI 请求失败，请稍后再试', life: 4000 })
}

function guardCodeTooLarge(): boolean {
  if (byteLength(code.value) <= MAX_CODE_BYTES) return false
  toast.add({ severity: 'warn', summary: '代码超过 64 KiB，无法使用 AI', life: 4000 })
  return true
}

async function loadAIConfig() {
  try {
    aiConfig.value = await getAIConfig()
    aiAvailable.value = true
    aiLoadFailed.value = false
  } catch (e) {
    aiConfig.value = null
    if (e instanceof ApiError && e.status === 404) {
      aiAvailable.value = false
      aiLoadFailed.value = false
    } else {
      aiAvailable.value = true
      aiLoadFailed.value = true
    }
  }
}

async function runComplete() {
  if (guardCodeTooLarge()) return
  const source = code.value
  const cursor = editorRef.value?.getCursorByteOffset() ?? byteLength(source)
  aiBusy.value = 'complete'
  try {
    const result = await completeCode({
      code: source,
      language: language.value,
      cursor,
      problemId: problemId.value,
    })
    const completion = result.completion
    if (!completion || !completion.trim()) {
      toast.add({ severity: 'info', summary: '没有可插入的补全', life: 3000 })
      return
    }
    editorRef.value?.insertAtCursor(completion)
  } catch (e) {
    handleAIError(e)
  } finally {
    aiBusy.value = null
  }
}

async function runFormat() {
  if (guardCodeTooLarge()) return
  const source = code.value
  aiBusy.value = 'format'
  try {
    const result = await formatCode({
      code: source,
      language: language.value,
      problemId: problemId.value,
    })
    if (result.code === source) {
      toast.add({ severity: 'info', summary: '代码已是格式化状态', life: 3000 })
      return
    }
    formatPreview.value = result.code
    formatDialogVisible.value = true
  } catch (e) {
    handleAIError(e)
  } finally {
    aiBusy.value = null
  }
}

function applyFormat() {
  code.value = formatPreview.value
  formatDialogVisible.value = false
}

async function runOptimize() {
  if (guardCodeTooLarge()) return
  const source = code.value
  aiBusy.value = 'optimize'
  try {
    const result = await optimizeCode({
      code: source,
      language: language.value,
      problemId: problemId.value,
    })
    optimizeSuggestions.value = result.suggestions
    optimizePreview.value = result.code
    optimizeDialogVisible.value = true
  } catch (e) {
    handleAIError(e)
  } finally {
    aiBusy.value = null
  }
}

function applyOptimize() {
  code.value = optimizePreview.value
  optimizeDialogVisible.value = false
}

function storageKey(lang: CodeSubmissionLanguage): string {
  const scope = draftContestId > 0 ? `contest:${draftContestId}` : 'practice'
  return `kapoj:code:${scope}:${draftProblemId}:${lang}`
}

function loadDraft(lang: CodeSubmissionLanguage) {
  try {
    drafts[lang] = localStorage.getItem(storageKey(lang)) ?? templates[lang]
  } catch {
    drafts[lang] = templates[lang]
  }
}

function saveDraft(lang: CodeSubmissionLanguage, value: string) {
  try {
    localStorage.setItem(storageKey(lang), value)
  } catch {
    // Private browsing or a full quota must not block submissions.
  }
}

let saveTimer: number | undefined

watch(code, () => {
  if (isObjective.value) return
  const lang = language.value
  const value = code.value
  if (saveTimer !== undefined) window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => {
    saveTimer = undefined
    saveDraft(lang, value)
  }, 500)
})

onBeforeUnmount(() => {
  if (saveTimer !== undefined) window.clearTimeout(saveTimer)
  if (!isObjective.value) saveDraft(language.value, code.value)
})

async function loadProblem() {
  loading.value = true
  error.value = ''
  try {
    problem.value = await getProblem(problemId.value)
  } catch (e) {
    error.value =
      e instanceof ApiError && e.code === 40400 ? '题目不存在' : '加载题目失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  // Drafts are keyed by problem/contest, not problem type. Load them before any
  // await so an early unmount flushes saved code instead of an empty draft.
  for (const lang of SUBMISSION_LANGUAGES) loadDraft(lang.value)

  if (route.query.contestId && contestId.value === 0) {
    toast.add({
      severity: 'warn',
      summary: '比赛参数无效',
      detail: '将按练习提交处理',
      life: 4000,
    })
  }

  const contestPromise =
    contestId.value > 0
      ? getContest(contestId.value)
          .then((c) => {
            contest.value = c
          })
          .catch(() => {
            contest.value = null
          })
      : Promise.resolve()

  await loadProblem()

  // Objective problems have no AI tools.
  if (!isObjective.value) void loadAIConfig()

  await contestPromise
})

async function submitAnswer() {
  const text = answer.value.trim()
  if (!text) {
    toast.add({ severity: 'warn', summary: '答案不能为空', life: 3000 })
    return
  }

  submitting.value = true
  try {
    const created = await createSubmission({
      problemId: problemId.value,
      ...(contestId.value > 0 ? { contestId: contestId.value } : {}),
      answer: text,
    })
    toast.add({
      severity: 'success',
      summary: `已提交 #${created.id}`,
      detail: '正在评测',
      life: 3000,
    })
    const query: Record<string, string> = { problemId: String(problemId.value) }
    if (contestId.value > 0) query.contestId = String(contestId.value)
    await router.push({ name: 'submissions', query })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '提交失败',
      detail: e instanceof ApiError ? e.message : '提交失败，请稍后再试',
      life: 4000,
    })
  } finally {
    submitting.value = false
  }
}

async function submit() {
  const lang = language.value
  const source = code.value
  if (!source.trim()) {
    toast.add({ severity: 'warn', summary: '代码不能为空', life: 3000 })
    return
  }

  saveDraft(lang, source)
  submitting.value = true
  try {
    const created = await createSubmission({
      problemId: problemId.value,
      contestId: contestId.value || undefined,
      language: lang,
      code: source,
    })
    toast.add({
      severity: 'success',
      summary: `已提交 #${created.id}`,
      detail: '正在评测',
      life: 3000,
    })
    const query: Record<string, string> = { problemId: String(problemId.value) }
    if (contestId.value > 0) query.contestId = String(contestId.value)
    await router.push({ name: 'submissions', query })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '提交失败',
      detail: e instanceof ApiError ? e.message : '提交失败，请稍后再试',
      life: 4000,
    })
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="submit-page">
    <AppSurface v-if="loading && !problem" padding="lg">
      <AppLoadingState label="正在加载题目" />
    </AppSurface>

    <div v-else-if="error" class="error-panel">
      <AppErrorState :description="error" @retry="loadProblem" />
      <Button as="router-link" to="/problems" label="返回题库" severity="secondary" outlined />
    </div>

    <div v-else-if="problem" class="submit-content">
      <AppPageHeader
        :eyebrow="`SUBMIT // ${problem?.slug.toUpperCase() ?? ''}`"
        :title="problem?.title ?? ''"
      >
        <template #actions>
          <Button
            v-if="contestId > 0"
            as="router-link"
            :to="{ name: 'contest-detail', params: { id: contestId } }"
            :label="`返回比赛${contestTitle ? `：${contestTitle}` : ''}`"
            icon="pi pi-arrow-left"
            severity="secondary"
            outlined
          />
          <Button
            as="router-link"
            :to="{ name: 'problem-detail', params: { id: problem?.id ?? 0 } }"
            label="返回题目"
            icon="pi pi-book"
            severity="secondary"
            text
          />
        </template>
      </AppPageHeader>

      <div class="meta-bar">
        <span v-if="contestId > 0" class="contest-chip">比赛提交</span>
        <span class="slug-chip">{{ problem?.slug }}</span>
        <AppDifficultyTag :difficulty="problem?.difficulty ?? 0" />
        <span class="type-chip">{{ typeLabels[problem?.type ?? 'traditional'] }}</span>
        <span v-if="problem?.type !== 'objective'" class="meta-item">
          <i class="pi pi-clock" aria-hidden="true" /> {{ problem?.timeLimitMs }} ms
        </span>
        <span v-if="problem?.type !== 'objective'" class="meta-item">
          <i class="pi pi-database" aria-hidden="true" /> {{ problem?.memoryLimitMb }} MB
        </span>
      </div>

      <AppSurface padding="lg" class="editor-card">
        <template v-if="isObjective">
          <div class="answer-field">
            <label class="field-label" for="objective-answer">你的答案</label>
            <Textarea
              id="objective-answer"
              v-model="answer"
              data-test="objective-answer"
              class="answer-input"
              rows="8"
              maxlength="2000"
              placeholder="请输入答案"
              fluid
            />
            <p class="hint">提交时会自动去除答案首尾空白。</p>
          </div>
          <div class="answer-actions">
            <span class="char-count">{{ answer.length }} / 2000</span>
            <Button
              type="button"
              label="提交答案"
              icon="pi pi-send"
              :loading="submitting"
              class="submit-button"
              @click="submitAnswer"
            />
          </div>
        </template>

        <template v-else>
          <Message
            v-if="contestAIDisabled"
            data-test="contest-ai-banner"
            severity="warn"
            :closable="false"
          >
            比赛期间已禁用 AI，管理员可在比赛设置中调整。
          </Message>
          <div class="editor-toolbar">
            <div class="language-field">
              <label class="field-label" for="submission-language">语言</label>
              <Select
                v-model="language"
                input-id="submission-language"
                :options="SUBMISSION_LANGUAGES"
                option-label="label"
                option-value="value"
                class="language-select"
              />
            </div>
            <div class="ai-toolbar">
              <Button
                type="button"
                label="AI 补全"
                icon="pi pi-sparkles"
                size="small"
                text
                data-test="ai-complete-button"
                :loading="aiBusy === 'complete'"
                :disabled="aiButtonsDisabled"
                @click="runComplete"
              />
              <Button
                type="button"
                label="AI 格式化"
                icon="pi pi-sparkles"
                size="small"
                text
                data-test="ai-format-button"
                :loading="aiBusy === 'format'"
                :disabled="aiButtonsDisabled"
                @click="runFormat"
              />
              <Button
                type="button"
                label="AI 优化"
                icon="pi pi-sparkles"
                size="small"
                text
                data-test="ai-optimize-button"
                :loading="aiBusy === 'optimize'"
                :disabled="aiButtonsDisabled"
                @click="runOptimize"
              />
            </div>
            <Button
              type="button"
              :label="`提交 ${languageLabel(language)} 代码`"
              icon="pi pi-send"
              :loading="submitting"
              class="submit-button"
              @click="submit"
            />
          </div>

          <Message v-if="!aiAvailable" severity="warn" :closable="false">AI 功能未启用</Message>
          <Message v-else-if="showKeyMissingBanner" severity="warn" :closable="false">
            管理员尚未配置 AI 密钥
          </Message>
          <Message v-else-if="showConfigBanner" severity="info" :closable="false">
            尚未配置 AI 模型，配置后可使用补全/格式化/优化
            <RouterLink to="/settings/ai" class="ai-config-link">去配置</RouterLink>
          </Message>

          <p class="hint">代码保存于本机浏览器；提交后可在测评记录中查看结果。</p>

          <div class="editor-wrap">
            <CodeEditor
              ref="editorRef"
              v-model="code"
              :language="editorLanguage(language)"
              height="520px"
            />
          </div>
        </template>
      </AppSurface>

      <Dialog
        v-model:visible="formatDialogVisible"
        header="格式化预览"
        modal
        :draggable="false"
        :style="{ width: 'min(92vw, 720px)' }"
      >
        <pre class="ai-preview">{{ formatPreview }}</pre>
        <template #footer>
          <Button label="取消" severity="secondary" text @click="formatDialogVisible = false" />
          <Button label="应用格式化" icon="pi pi-check" @click="applyFormat" />
        </template>
      </Dialog>

      <Dialog
        v-model:visible="optimizeDialogVisible"
        header="优化建议"
        modal
        :draggable="false"
        :style="{ width: 'min(92vw, 720px)' }"
      >
        <MarkdownView :source="optimizeSuggestions" :theme="isDark ? 'dark' : 'light'" />
        <pre class="ai-preview">{{ optimizePreview }}</pre>
        <template #footer>
          <Button label="取消" severity="secondary" text @click="optimizeDialogVisible = false" />
          <Button
            label="应用优化代码"
            icon="pi pi-check"
            :disabled="!optimizeCanApply"
            @click="applyOptimize"
          />
        </template>
      </Dialog>
    </div>
  </div>
</template>

<style scoped>
.submit-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.submit-content {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.error-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  align-items: flex-start;
}

.meta-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.contest-chip,
.slug-chip,
.type-chip {
  padding: 1px 8px;
  font-size: 11px;
  border-radius: var(--kapoj-radius-xs);
}

.contest-chip {
  color: var(--kapoj-success);
  border: 1px solid color-mix(in srgb, var(--kapoj-success) 32%, transparent);
}

.slug-chip {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  border: 1px solid var(--kapoj-border);
}

.type-chip {
  color: var(--kapoj-info);
  border: 1px solid color-mix(in srgb, var(--kapoj-info) 32%, transparent);
}

.meta-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.meta-item i {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.editor-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.answer-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.answer-input {
  width: 100%;
  font-family: var(--kapoj-font-mono);
  font-size: 14px;
  line-height: 1.6;
}

.answer-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.char-count {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.editor-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.language-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 180px;
}

.field-label {
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.language-select {
  width: 180px;
}

.submit-button {
  min-width: 180px;
}

.ai-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.ai-config-link {
  margin-left: var(--kapoj-space-2);
  color: var(--kapoj-info);
  font-weight: 600;
  text-decoration: none;
}

.ai-config-link:hover {
  text-decoration: underline;
}

.ai-preview {
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

.hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.editor-wrap {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

@media (max-width: 600px) {
  .editor-toolbar {
    align-items: stretch;
  }

  .language-field {
    width: 100%;
    min-width: 0;
  }

  .language-field :deep(.p-select),
  .submit-button {
    width: 100%;
  }

  .submit-button {
    min-width: 0;
  }
}
</style>
