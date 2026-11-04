<script setup lang="ts">
import { ref, shallowRef, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

import { ApiError } from '@/api/http'
import {
  deleteTestcase,
  listTestcases,
  reorderTestcases,
  updateTestcase,
  uploadChecker,
  uploadTestcase,
} from '@/api/problems'
import type { ProblemType, Testcase } from '@/api/types'
import { formatBytes } from '@/utils/format'

const props = defineProps<{ problemId: number; type: ProblemType; checkerUploaded: boolean }>()
const emit = defineEmits<{ (e: 'changed'): void }>()

const cases = ref<Testcase[]>([])
const loading = ref(false)
const busy = ref(false)

const inputFile = shallowRef<File | null>(null)
const outputFile = shallowRef<File | null>(null)
const checkerFile = shallowRef<File | null>(null)
const score = ref(10)
const isSample = ref(false)
const inputEl = ref<HTMLInputElement>()
const outputEl = ref<HTMLInputElement>()
const checkerEl = ref<HTMLInputElement>()

function fail(e: unknown, fallback: string) {
  ElMessage.error(e instanceof ApiError ? e.message : fallback)
}

// Only the newest request may update the table; switching problems quickly
// must not leave the previous problem's testcases on screen.
let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  cases.value = []
  try {
    const list = await listTestcases(props.problemId)
    if (seq === latest) cases.value = list
  } catch (e) {
    if (seq === latest) fail(e, '加载测试点失败')
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  () => props.problemId,
  () => void load(),
  { immediate: true },
)

function fileOf(e: Event): File | null {
  return (e.target as HTMLInputElement).files?.[0] ?? null
}

async function run(action: () => Promise<unknown>, failure: string): Promise<boolean> {
  busy.value = true
  try {
    await action()
    return true
  } catch (e) {
    fail(e, failure)
    return false
  } finally {
    busy.value = false
  }
}

async function upload() {
  if (!inputFile.value || !outputFile.value) {
    ElMessage.warning('请同时选择输入文件和输出文件')
    return
  }
  const ok = await run(
    () =>
      uploadTestcase(props.problemId, {
        input: inputFile.value!,
        output: outputFile.value!,
        score: score.value,
        isSample: isSample.value,
      }),
    '上传失败',
  )
  if (!ok) return
  inputFile.value = outputFile.value = null
  for (const el of [inputEl.value, outputEl.value]) if (el) el.value = ''
  ElMessage.success('测试点已上传')
  await load()
  emit('changed')
}

async function changeScore(tc: Testcase, value: number | undefined) {
  if (value === undefined || value === tc.score) return
  if (await run(() => updateTestcase(props.problemId, tc.id, { score: value }), '修改分数失败')) {
    tc.score = value
  } else {
    await load()
  }
}

async function changeSample(tc: Testcase, value: boolean) {
  if (
    await run(() => updateTestcase(props.problemId, tc.id, { isSample: value }), '修改样例标记失败')
  ) {
    tc.isSample = value
  } else {
    await load()
  }
}

async function move(index: number, delta: -1 | 1) {
  const next = [...cases.value]
  const target = index + delta
  if (target < 0 || target >= next.length) return
  ;[next[index], next[target]] = [next[target], next[index]]
  if (
    await run(
      () =>
        reorderTestcases(
          props.problemId,
          next.map((c) => c.id),
        ),
      '调整顺序失败',
    )
  ) {
    cases.value = next.map((c, i) => ({ ...c, sortOrder: i + 1 }))
  } else {
    await load()
  }
}

async function remove(tc: Testcase) {
  try {
    await ElMessageBox.confirm(`确定删除第 ${tc.sortOrder} 个测试点吗？`, '删除测试点', {
      type: 'warning',
    })
  } catch {
    return
  }
  if (await run(() => deleteTestcase(props.problemId, tc.id), '删除失败')) {
    ElMessage.success('已删除')
    await load()
    emit('changed')
  }
}

async function sendChecker() {
  if (!checkerFile.value) {
    ElMessage.warning('请先选择 checker 源码文件')
    return
  }
  if (await run(() => uploadChecker(props.problemId, checkerFile.value!), 'checker 上传失败')) {
    ElMessage.success('checker 已上传')
    checkerFile.value = null
    if (checkerEl.value) checkerEl.value.value = ''
    emit('changed')
  }
}
</script>

<template>
  <div>
    <h3>测试点</h3>
    <el-table v-loading="loading" :data="cases" empty-text="还没有测试点" size="small">
      <el-table-column prop="sortOrder" label="序号" width="60" />
      <el-table-column label="大小（输入 / 输出）" min-width="140">
        <template #default="{ row }">
          {{ formatBytes(row.inputSize) }} / {{ formatBytes(row.outputSize) }}
        </template>
      </el-table-column>
      <el-table-column label="分数" width="140">
        <template #default="{ row }">
          <el-input-number
            :model-value="row.score"
            :min="0"
            :max="1000"
            size="small"
            :disabled="busy"
            @change="(v: number | undefined) => changeScore(row, v)"
          />
        </template>
      </el-table-column>
      <el-table-column label="样例" width="70">
        <template #default="{ row }">
          <el-switch
            :model-value="row.isSample"
            :disabled="busy"
            @change="(v: boolean | string | number) => changeSample(row, Boolean(v))"
          />
        </template>
      </el-table-column>
      <el-table-column label="操作" min-width="170">
        <template #default="{ row, $index }">
          <div class="ops">
            <el-button text size="small" :disabled="busy || $index === 0" @click="move($index, -1)"
              >上移</el-button
            >
            <el-button
              text
              size="small"
              :disabled="busy || $index === cases.length - 1"
              @click="move($index, 1)"
              >下移</el-button
            >
            <el-button text size="small" type="danger" :disabled="busy" @click="remove(row)"
              >删除</el-button
            >
          </div>
        </template>
      </el-table-column>
    </el-table>

    <h4>上传新测试点</h4>
    <div class="upload">
      <label
        >输入文件 <input ref="inputEl" type="file" @change="inputFile = fileOf($event)"
      /></label>
      <label
        >输出文件 <input ref="outputEl" type="file" @change="outputFile = fileOf($event)"
      /></label>
      <label>分数 <el-input-number v-model="score" :min="0" :max="1000" size="small" /></label>
      <el-checkbox v-model="isSample">作为样例</el-checkbox>
      <el-button type="primary" :loading="busy" @click="upload">上传测试点</el-button>
    </div>
    <p class="hint">输入、输出文件各不超过 16 MiB。</p>

    <template v-if="type === 'special'">
      <h3>特判 checker</h3>
      <p>
        状态：
        <el-tag :type="checkerUploaded ? 'success' : 'warning'">
          {{ checkerUploaded ? '已上传' : '未上传' }}
        </el-tag>
      </p>
      <div class="upload">
        <label
          >checker 源码（C++，不超过 256 KiB）
          <input
            ref="checkerEl"
            type="file"
            accept=".cpp,.cc,.cxx,.txt"
            @change="checkerFile = fileOf($event)"
          />
        </label>
        <el-button :loading="busy" @click="sendChecker">上传 checker</el-button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ops {
  display: flex;
  white-space: nowrap;
}
.ops .el-button + .el-button {
  margin-left: 0;
}
.upload {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 20px;
  align-items: center;
}
.hint {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
</style>
