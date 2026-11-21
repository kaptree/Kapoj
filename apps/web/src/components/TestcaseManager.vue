<script setup lang="ts">
import { computed, ref, shallowRef, watch } from 'vue'
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
  uploadTestcasesBatch,
} from '@/api/problems'
import type { ProblemType, Testcase } from '@/api/types'
import TestcaseEditDialog from '@/components/TestcaseEditDialog.vue'
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

const editVisible = ref(false)
const editing = ref<Testcase | null>(null)

const batchFiles = ref<File[]>([])
const batchScore = ref(10)
const batchUploading = ref(false)
const batchDragActive = ref(false)
const batchInput = ref<HTMLInputElement>()
const batchError = ref('')

interface BatchPairPreview {
  name: string
  input: string
  output: string
  ok: boolean
}

const batchPreview = computed<BatchPairPreview[]>(() => {
  type Entry = { input?: File; out?: File; ans?: File }
  const entries = new Map<string, Entry>()
  for (const file of batchFiles.value) {
    const lower = file.name.toLowerCase()
    let name = ''
    let slot: keyof Entry | null = null
    if (lower.endsWith('.in')) {
      name = file.name.slice(0, -3)
      slot = 'input'
    } else if (lower.endsWith('.out')) {
      name = file.name.slice(0, -4)
      slot = 'out'
    } else if (lower.endsWith('.ans')) {
      name = file.name.slice(0, -4)
      slot = 'ans'
    }
    if (!name || !slot) continue
    const entry = entries.get(name) ?? {}
    entry[slot] = file
    entries.set(name, entry)
  }
  return [...entries.entries()]
    .sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true }))
    .map(([name, entry]) => {
      const output = entry.out ?? entry.ans
      return {
        name,
        input: entry.input?.name ?? '缺少 .in',
        output: output?.name ?? '缺少 .out/.ans',
        ok: Boolean(entry.input && output),
      }
    })
})

const batchValid = computed(
  () => batchPreview.value.length > 0 && batchPreview.value.every((pair) => pair.ok),
)

function openCreate() {
  editing.value = null
  editVisible.value = true
}

function openEdit(tc: Testcase) {
  editing.value = tc
  editVisible.value = true
}

async function onEdited() {
  await load()
  emit('changed')
}

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

function addBatchFiles(files: File[]) {
  batchError.value = ''
  batchFiles.value = [...batchFiles.value, ...files]
}

function onBatchChange(event: Event) {
  const input = event.target as HTMLInputElement
  addBatchFiles([...(input.files ?? [])])
  input.value = ''
}

function onBatchDrop(event: DragEvent) {
  batchDragActive.value = false
  addBatchFiles([...(event.dataTransfer?.files ?? [])])
}

function clearBatch() {
  batchFiles.value = []
  batchError.value = ''
  if (batchInput.value) batchInput.value.value = ''
}

async function uploadBatch() {
  if (!batchValid.value || batchUploading.value) {
    batchError.value = '每个 .in 必须对应一个 .out 或 .ans'
    return
  }
  batchUploading.value = true
  batchError.value = ''
  try {
    await uploadTestcasesBatch(props.problemId, batchFiles.value, batchScore.value)
    toast.add({ severity: 'success', summary: '批量测试点已上传', life: 3000 })
    clearBatch()
    await load()
    emit('changed')
  } catch (e) {
    batchError.value = e instanceof ApiError ? e.message : '批量上传失败，请稍后再试'
  } finally {
    batchUploading.value = false
  }
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
      <div class="tc-header__actions">
        <Button label="在线创建测试点" text :disabled="busy" @click="openCreate" />
        <Button
          v-if="type === 'traditional'"
          label="AI 造数据"
          text
          :disabled="busy"
          @click="goAiGenerate"
        />
      </div>
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
                label="查看/编辑"
                text
                size="small"
                :disabled="busy"
                @click="openEdit(data)"
              />
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
      <p class="hint">输入、输出文件各受评测设置限制（默认各 16 MiB）。</p>
    </section>

    <section class="uploader batch-uploader">
      <h3>批量上传测试数据</h3>
      <div
        class="batch-drop"
        :class="{ 'batch-drop--active': batchDragActive }"
        role="button"
        tabindex="0"
        data-test="batch-drop"
        @click="batchInput?.click()"
        @keydown.enter.prevent="batchInput?.click()"
        @keydown.space.prevent="batchInput?.click()"
        @dragover.prevent="batchDragActive = true"
        @dragleave.prevent="batchDragActive = false"
        @drop.prevent="onBatchDrop"
      >
        <i class="pi pi-cloud-upload" aria-hidden="true" />
        <span>点击或拖入多个 .in / .out / .ans 文件</span>
        <small>每个 .in 必须对应一个 .out 或 .ans；同名同时存在时使用 .out</small>
        <input
          ref="batchInput"
          type="file"
          multiple
          class="file-input"
          accept=".in,.out,.ans"
          data-test="batch-input"
          @click.stop
          @change="onBatchChange"
        />
      </div>

      <ul v-if="batchPreview.length" class="batch-preview" data-test="batch-preview">
        <li v-for="pair in batchPreview" :key="pair.name" :class="{ 'is-bad': !pair.ok }">
          <span class="mono">{{ pair.name }}</span>
          <span>{{ pair.input }}</span>
          <span>↔</span>
          <span>{{ pair.output }}</span>
          <span class="batch-preview__status">{{ pair.ok ? '已配对' : '未配对' }}</span>
        </li>
      </ul>

      <Message v-if="batchError" severity="error" :closable="false">{{ batchError }}</Message>

      <div class="batch-actions">
        <label class="score-field">
          分数
          <InputNumber v-model="batchScore" :min="0" :max="1000" size="small" />
        </label>
        <Button
          label="清空"
          severity="secondary"
          text
          :disabled="batchUploading"
          @click="clearBatch"
        />
        <Button
          label="批量上传"
          :loading="batchUploading"
          :disabled="!batchValid || batchUploading"
          @click="uploadBatch"
        />
      </div>
      <p class="hint">一次最多 100 对；单文件默认 16 MiB，总量默认 200 MiB，管理员可调整。</p>
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

    <TestcaseEditDialog
      :visible="editVisible"
      :problem-id="problemId"
      :testcase="editing"
      @update:visible="editVisible = $event"
      @saved="onEdited"
    />
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

.tc-header__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--kapoj-space-1);
}

.batch-drop {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  text-align: center;
  cursor: pointer;
  border: 1px dashed var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.batch-drop:hover,
.batch-drop--active {
  color: var(--kapoj-primary);
  border-color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.batch-drop i {
  font-size: 1.5rem;
}

.batch-drop small {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.batch-preview {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 220px;
  padding: 0;
  margin: 0;
  overflow-y: auto;
  list-style: none;
}

.batch-preview li {
  display: grid;
  grid-template-columns: 70px minmax(0, 1fr) 16px minmax(0, 1fr) 58px;
  gap: var(--kapoj-space-1);
  align-items: center;
  padding: 4px 6px;
  font-size: 11px;
  border-radius: var(--kapoj-radius-sm);
  background: var(--kapoj-bg-elevated);
}

.batch-preview li.is-bad {
  color: var(--kapoj-danger);
}

.batch-preview__status {
  color: var(--kapoj-text-muted);
  text-align: right;
}

.mono {
  font-family: var(--kapoj-font-mono);
}

.batch-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
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
