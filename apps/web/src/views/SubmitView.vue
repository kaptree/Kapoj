<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import { getContest } from '@/api/contests'
import { ApiError } from '@/api/http'
import { getProblem } from '@/api/problems'
import { createSubmission } from '@/api/submissions'
import type { ProblemDetail, SubmissionLanguage } from '@/api/types'
import CodeEditor from '@/components/CodeEditor.vue'
import AppDifficultyTag from '@/components/ui/AppDifficultyTag.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { languageLabel, monacoLanguage, SUBMISSION_LANGUAGES } from '@/utils/submission'

const route = useRoute()
const router = useRouter()
const toast = useToast()

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
const contestTitle = ref('')
const loading = ref(false)
const submitting = ref(false)
const error = ref('')
const language = ref<SubmissionLanguage>('cpp')
const drafts = reactive<Record<SubmissionLanguage, string>>({
  c: '',
  cpp: '',
  python3: '',
  java: '',
  go: '',
})

const templates: Record<SubmissionLanguage, string> = {
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
} as const

const code = computed({
  get: () => drafts[language.value],
  set: (value: string) => {
    drafts[language.value] = value
  },
})

function storageKey(lang: SubmissionLanguage): string {
  const scope = draftContestId > 0 ? `contest:${draftContestId}` : 'practice'
  return `kapoj:code:${scope}:${draftProblemId}:${lang}`
}

function loadDraft(lang: SubmissionLanguage) {
  try {
    drafts[lang] = localStorage.getItem(storageKey(lang)) ?? templates[lang]
  } catch {
    drafts[lang] = templates[lang]
  }
}

function saveDraft(lang: SubmissionLanguage, value: string) {
  try {
    localStorage.setItem(storageKey(lang), value)
  } catch {
    // Private browsing or a full quota must not block submissions.
  }
}

let saveTimer: number | undefined

watch(code, () => {
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
  saveDraft(language.value, code.value)
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
  for (const lang of SUBMISSION_LANGUAGES) loadDraft(lang.value)

  if (route.query.contestId && contestId.value === 0) {
    toast.add({
      severity: 'warn',
      summary: '比赛参数无效',
      detail: '将按练习提交处理',
      life: 4000,
    })
  }

  const titlePromise =
    contestId.value > 0
      ? getContest(contestId.value)
          .then((c) => c.title)
          .catch(() => '')
      : Promise.resolve('')

  await loadProblem()
  contestTitle.value = await titlePromise
})

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
        <span class="meta-item">
          <i class="pi pi-clock" aria-hidden="true" /> {{ problem?.timeLimitMs }} ms
        </span>
        <span class="meta-item">
          <i class="pi pi-database" aria-hidden="true" /> {{ problem?.memoryLimitMb }} MB
        </span>
      </div>

      <AppSurface padding="lg" class="editor-card">
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
          <Button
            type="button"
            :label="`提交 ${languageLabel(language)} 代码`"
            icon="pi pi-send"
            :loading="submitting"
            class="submit-button"
            @click="submit"
          />
        </div>

        <p class="hint">代码保存于本机浏览器；提交后可在测评记录中查看结果。</p>

        <div class="editor-wrap">
          <CodeEditor v-model="code" :language="monacoLanguage(language)" height="520px" />
        </div>
      </AppSurface>
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
