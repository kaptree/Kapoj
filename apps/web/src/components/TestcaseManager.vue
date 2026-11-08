<script setup lang="ts">
import { ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

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

const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

const cases = ref<Testcase[]>([])
const loading = ref(false)
const busy = ref(false)
const loadError = ref('')

const inputFile = shallowRef<File | null>(null)
const outputFile = shallowRef<File | null>(null)
const checkerFile = shallowRef<File | null>(null)
const score = ref(10)
const isSample = ref(false)
const inputEl = ref<HTMLInputElement>()
const outputEl = ref<HTMLInputElement>()
const checkerEl = ref<HTMLInputElement>()

function fail(e: unknown, fallback: string) {
  toast.add({
    severity: 'error',
    summary: e instanceof ApiError ? e.message : fallback,
    life: 4000,
  })
}

// Only the newest request may update the table; switching problems quickly
// must not leave the previous problem's testcases on screen.
let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  cases.value = []
  loadError.value = ''
  try {
    const list = await listTestcases(props.problemId)
    if (seq === latest) cases.value = list
  } catch (e) {
    if (seq === latest) loadError.value = e instanceof ApiError ? e.message : '加载测试点失败'
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
    toast.add({ severity: 'warn', summary: '请同时选择输入文件和输出文件', life: 3000 })
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
  toast.add({ severity: 'success', summary: '测试点已上传', life: 3000 })
  await load()
  emit('changed')
}

async function changeScore(tc: Testcase, value: number | null | undefined) {
  if (value == null || value === tc.score) return
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
  confirm.require({
    header: '删除测试点',
    message: `确定删除第 ${tc.sortOrder} 个测试点吗？`,
    acceptLabel: '删除',
    rejectLabel: '取消',
    accept: async () => {
      if (await run(() => deleteTestcase(props.problemId, tc.id), '删除失败')) {
        toast.add({ severity: 'success', summary: '已删除', life: 3000 })
        await load()
        emit('changed')
      }
    },
  })
}

async function sendChecker() {
  if (!checkerFile.value) {
    toast.add({ severity: 'warn', summary: '请先选择 checker 源码文件', life: 3000 })
    return
  }
  if (await run(() => uploadChecker(props.problemId, checkerFile.value!), 'checker 上传失败')) {
    toast.add({ severity: 'success', summary: 'checker 已上传', life: 3000 })
    checkerFile.value = null
    if (checkerEl.value) checkerEl.value.value = ''
    emit('changed')
  }
}

function goAiGenerate() {
  void router.push({ name: 'problem-testcase-generator', params: { id: props.problemId } })
}
</script>

<template>
  <div class="tc">
    <div class="tc-header">
      <h2>测试点</h2>
      <Button
        v-if="type === 'traditional'"
        label="AI 造数据"
        text
        :disabled="busy"
        @click="goAiGenerate"
      />
    </div>

    <Message v-if="loadError" severity="error" :closable="false">
      <div class="load-error">
        <span>{{ loadError }}</span>
        <Button label="重试" text size="small" @click="load" />
      </div>
    </Message>

    <div class="tc-table-wrap" tabindex="0" role="region" aria-label="测试点表格，可横向滚动">
      <DataTable :value="cases" :loading="loading" data-key="id" size="small" class="tc-table">
        <Column header="序号" style="width: 60px">
          <template #body="{ data }">{{ data.sortOrder }}</template>
        </Column>
        <Column header="大小（输入 / 输出）" style="min-width: 140px">
          <template #body="{ data }">
            {{ formatBytes(data.inputSize) }} / {{ formatBytes(data.outputSize) }}
          </template>
        </Column>
        <Column header="分数" style="width: 140px">
          <template #body="{ data }">
            <InputNumber
              :model-value="data.score"
              :min="0"
              :max="1000"
              size="small"
              :disabled="busy"
              @value-change="(v) => changeScore(data, v)"
            />
          </template>
        </Column>
        <Column header="样例" style="width: 80px">
          <template #body="{ data }">
            <ToggleSwitch
              :model-value="data.isSample"
              :disabled="busy"
              @update:model-value="(v: boolean) => changeSample(data, v)"
            />
          </template>
        </Column>
        <Column header="操作" style="min-width: 170px">
          <template #body="{ data, index }">
            <div class="ops">
              <Button
                label="上移"
                text
                size="small"
                :disabled="busy || index === 0"
                @click="move(index, -1)"
              />
              <Button
                label="下移"
                text
                size="small"
                :disabled="busy || index === cases.length - 1"
                @click="move(index, 1)"
              />
              <Button
                label="删除"
                text
                size="small"
                severity="danger"
                :disabled="busy"
                @click="remove(data)"
              />
            </div>
          </template>
        </Column>
        <template #empty>还没有测试点</template>
      </DataTable>
    </div>

    <section class="uploader">
      <h3>上传新测试点</h3>
      <div class="upload">
        <input ref="inputEl" type="file" class="file-input" @change="inputFile = fileOf($event)" />
        <Button label="选择输入文件" text size="small" @click="inputEl?.click()" />
        <span class="file-name">{{ inputFile?.name || '未选择' }}</span>

        <input
          ref="outputEl"
          type="file"
          class="file-input"
          @change="outputFile = fileOf($event)"
        />
        <Button label="选择输出文件" text size="small" @click="outputEl?.click()" />
        <span class="file-name">{{ outputFile?.name || '未选择' }}</span>

        <label class="score-field">
          分数
          <InputNumber v-model="score" :min="0" :max="1000" size="small" />
        </label>

        <div class="sample-field">
          <ToggleSwitch v-model="isSample" :disabled="busy" input-id="tc-sample" />
          <label for="tc-sample">作为样例</label>
        </div>

        <Button
          label="上传测试点"
          :loading="busy"
          :disabled="!inputFile || !outputFile"
          @click="upload"
        />
      </div>
      <p class="hint">输入、输出文件各不超过 16 MiB。</p>
    </section>

    <template v-if="type === 'special'">
      <section class="checker">
        <h3>特判 checker</h3>
        <div class="checker-status">
          <span>状态：</span>
          <Tag
            :value="checkerUploaded ? '已上传' : '未上传'"
            :severity="checkerUploaded ? 'success' : 'warn'"
          />
        </div>
        <div class="upload">
          <input
            ref="checkerEl"
            type="file"
            class="file-input"
            accept=".cpp,.cc,.cxx,.txt"
            @change="checkerFile = fileOf($event)"
          />
          <Button label="选择 checker" text size="small" @click="checkerEl?.click()" />
          <span class="file-name">{{ checkerFile?.name || '未选择' }}</span>
          <Button label="上传 checker" :loading="busy" @click="sendChecker" />
        </div>
        <p class="hint">checker 源码（C++，不超过 256 KiB）。</p>
      </section>
    </template>
  </div>
</template>

<style scoped>
.tc {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.tc-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.tc-header h2 {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.load-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.tc-table-wrap {
  overflow-x: auto;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.tc-table-wrap:focus-visible {
  outline: 2px solid var(--kapoj-primary);
  outline-offset: 2px;
}

/* The wrapper owns horizontal scrolling on narrow screens; avoid nesting a
   second scroll container inside DataTable. */
.tc-table-wrap :deep(.p-datatable-table-container) {
  overflow: visible !important;
}

.tc-table :deep(.p-datatable-table) {
  min-width: 590px;
}

.tc-table :deep(.p-datatable-thead > tr > th) {
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

.tc-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.ops {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
}

.uploader,
.checker {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
}

.uploader h3,
.checker h3 {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.875rem;
  font-weight: 600;
}

.upload {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 20px;
  align-items: center;
}

.file-input {
  display: none;
}

.file-name {
  min-width: 8ch;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.score-field {
  display: inline-flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.score-field :deep(.p-inputnumber) {
  width: 7rem;
}

.sample-field {
  display: inline-flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.sample-field label {
  color: var(--kapoj-text-secondary);
  font-size: 13px;
  cursor: pointer;
}

.checker-status {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 13px;
}
</style>
