<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'

import { ApiError } from '@/api/http'
import { getProblem } from '@/api/problems'
import { createSubmission } from '@/api/submissions'
import type { ProblemDetail, SubmissionLanguage } from '@/api/types'
import CodeEditor from '@/components/CodeEditor.vue'
import { languageLabel, monacoLanguage, SUBMISSION_LANGUAGES } from '@/utils/submission'

const route = useRoute()
const router = useRouter()
const problemId = computed(() => Number(route.params.id))

const problem = ref<ProblemDetail | null>(null)
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

const code = computed({
  get: () => drafts[language.value],
  set: (value: string) => {
    drafts[language.value] = value
  },
})

function storageKey(lang: SubmissionLanguage): string {
  return `kapoj:code:${problemId.value}:${lang}`
}

function loadDraft(lang: SubmissionLanguage) {
  drafts[lang] = localStorage.getItem(storageKey(lang)) ?? templates[lang]
}

onMounted(async () => {
  for (const lang of SUBMISSION_LANGUAGES) loadDraft(lang.value)
  loading.value = true
  try {
    problem.value = await getProblem(problemId.value)
  } catch (e) {
    error.value =
      e instanceof ApiError && e.code === 40400 ? '题目不存在' : '加载题目失败，请稍后再试'
  } finally {
    loading.value = false
  }
})

async function submit() {
  if (!code.value.trim()) {
    ElMessage.warning('代码不能为空')
    return
  }
  submitting.value = true
  try {
    const created = await createSubmission({
      problemId: problemId.value,
      language: language.value,
      code: code.value,
    })
    localStorage.setItem(storageKey(language.value), code.value)
    ElMessage.success(`已提交 #${created.id}，正在评测`)
    await router.push({ name: 'submissions', query: { problemId: String(problemId.value) } })
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '提交失败，请稍后再试')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div v-loading="loading" class="page">
    <el-result v-if="error" icon="warning" :title="error">
      <template #extra>
        <RouterLink :to="{ name: 'problems' }">
          <el-button type="primary">返回题库</el-button>
        </RouterLink>
      </template>
    </el-result>

    <template v-else-if="problem">
      <el-card shadow="never" class="head-card">
        <div class="head">
          <div>
            <h1>{{ problem.title }}</h1>
            <div class="meta">
              <el-tag size="small">{{ problem.slug }}</el-tag>
              <span>时间限制 {{ problem.timeLimitMs }} ms</span>
              <span>内存限制 {{ problem.memoryLimitMb }} MB</span>
            </div>
          </div>
          <RouterLink :to="{ name: 'problem-detail', params: { id: problem.id } }">
            <el-button>返回题目</el-button>
          </RouterLink>
        </div>
      </el-card>

      <el-card shadow="never" class="editor-card">
        <div class="toolbar">
          <el-select v-model="language" style="width: 150px">
            <el-option
              v-for="item in SUBMISSION_LANGUAGES"
              :key="item.value"
              :label="item.label"
              :value="item.value"
            />
          </el-select>
          <el-button type="primary" :loading="submitting" @click="submit">
            提交 {{ languageLabel(language) }} 代码
          </el-button>
        </div>
        <p class="hint">代码保存于本机浏览器；提交后可在测评记录中查看结果。</p>
        <CodeEditor v-model="code" :language="monacoLanguage(language)" height="520px" />
      </el-card>
    </template>
  </div>
</template>

<style scoped>
.page {
  min-height: 200px;
}
.head-card {
  margin-bottom: 16px;
}
.head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
h1 {
  margin: 0;
  font-size: 22px;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 8px;
  color: var(--el-text-color-secondary);
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}
.hint {
  margin: 0 0 12px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
</style>
