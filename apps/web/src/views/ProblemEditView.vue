<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'

import { ApiError } from '@/api/http'
import { createProblem, deleteProblem, getProblem, updateProblem } from '@/api/problems'
import type { ProblemDetail, ProblemInput } from '@/api/types'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import SampleEditor from '@/components/SampleEditor.vue'
import TestcaseManager from '@/components/TestcaseManager.vue'
import { DIFFICULTIES } from '@/utils/difficulty'

const route = useRoute()
const router = useRouter()

const problemId = computed(() =>
  typeof route.params.id === 'string' ? Number(route.params.id) : null,
)
const isEdit = computed(() => problemId.value !== null)

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
  try {
    fill(await getProblem(problemId.value))
  } catch (e) {
    loadFailed.value = true
    error.value =
      e instanceof ApiError && e.code === 40400 ? '题目不存在' : '加载题目失败，请稍后再试'
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
      ElMessage.success('已保存')
    } else {
      const created = await createProblem(form)
      ElMessage.success('题目已创建。请上传测试点，再回到这里发布。')
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

async function remove() {
  if (problemId.value === null) return
  try {
    await ElMessageBox.confirm('确定删除这道题目吗？已有提交记录的题目只会被下线。', '删除题目', {
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    const { offlined } = await deleteProblem(problemId.value)
    ElMessage.success(offlined ? '该题已有提交记录，已下线而不是删除' : '题目已删除')
    await router.replace({ name: 'problems' })
  } catch (e) {
    error.value = describe(e, '删除失败，请稍后再试')
  }
}
</script>

<template>
  <div v-loading="loading" class="page">
    <el-result v-if="loadFailed" icon="warning" :title="error">
      <template #extra>
        <RouterLink :to="{ name: 'problems' }"
          ><el-button type="primary">返回题库</el-button></RouterLink
        >
      </template>
    </el-result>

    <el-card v-else shadow="never">
      <h1>{{ isEdit ? '编辑题目' : '新建题目' }}</h1>
      <el-alert
        v-if="error"
        :title="error"
        type="error"
        show-icon
        :closable="false"
        class="alert"
      />
      <el-alert
        v-if="!isEdit"
        title="新题目创建后默认不公开。创建后上传测试点，再回到这里发布。"
        type="info"
        show-icon
        :closable="false"
        class="alert"
      />

      <el-form :model="form" label-width="96px" @submit.prevent="save">
        <el-form-item label="标题" required>
          <el-input v-model="form.title" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item label="题目标识" required>
          <el-input v-model="form.slug" placeholder="小写字母、数字和连字符，例如 a-plus-b" />
        </el-form-item>
        <el-form-item label="类型">
          <el-radio-group v-model="form.type">
            <el-radio value="traditional">传统题</el-radio>
            <el-radio value="special">特判题</el-radio>
            <el-radio value="interactive">交互题（暂不评测）</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="难度">
          <el-select v-model="form.difficulty" style="width: 140px">
            <el-option v-for="d in DIFFICULTIES" :key="d.value" :label="d.label" :value="d.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="时间限制">
          <el-input-number v-model="form.timeLimitMs" :min="100" :max="10000" :step="100" />
          <span class="unit">毫秒</span>
        </el-form-item>
        <el-form-item label="内存限制">
          <el-input-number v-model="form.memoryLimitMb" :min="16" :max="1024" :step="16" />
          <span class="unit">MB</span>
        </el-form-item>
        <el-form-item label="标签">
          <el-select
            v-model="form.tags"
            multiple
            filterable
            allow-create
            default-first-option
            :multiple-limit="10"
            placeholder="输入后回车添加，最多 10 个"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="来源">
          <el-input v-model="form.source" maxlength="200" />
        </el-form-item>

        <el-form-item label="题目内容" class="stack">
          <el-tabs type="border-card" class="tabs">
            <el-tab-pane label="题目描述" lazy
              ><MarkdownEditor v-model="form.statementMd"
            /></el-tab-pane>
            <el-tab-pane label="输入格式" lazy
              ><MarkdownEditor v-model="form.inputFormatMd" height="240px"
            /></el-tab-pane>
            <el-tab-pane label="输出格式" lazy
              ><MarkdownEditor v-model="form.outputFormatMd" height="240px"
            /></el-tab-pane>
            <el-tab-pane label="样例" lazy><SampleEditor v-model="form.samples" /></el-tab-pane>
            <el-tab-pane label="提示" lazy
              ><MarkdownEditor v-model="form.hintMd" height="240px"
            /></el-tab-pane>
          </el-tabs>
        </el-form-item>

        <el-form-item label="公开">
          <el-switch v-model="form.isPublic" :disabled="!isEdit" />
          <span class="unit">发布需要至少 1 个测试点；特判题还需要 checker。</span>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" native-type="submit" :loading="saving">
            {{ isEdit ? '保存修改' : '创建题目' }}
          </el-button>
          <RouterLink :to="{ name: 'problems' }"><el-button>返回题库</el-button></RouterLink>
          <el-button v-if="isEdit" type="danger" plain @click="remove">删除题目</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card v-if="isEdit && !loadFailed" shadow="never" class="testcases">
      <TestcaseManager
        :problem-id="problemId!"
        :type="form.type"
        :checker-uploaded="checkerUploaded"
        @changed="reloadFlags"
      />
    </el-card>
  </div>
</template>

<style scoped>
.page {
  min-height: 200px;
}
.alert {
  margin-bottom: 12px;
}
.unit {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
}
.tabs {
  width: 100%;
}
.testcases {
  margin-top: 16px;
}
</style>
