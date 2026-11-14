<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import {
  createOnlineTestcase,
  getTestcaseContent,
  updateTestcaseContent,
} from '@/api/problems'
import type { Testcase } from '@/api/types'
import { formatBytes } from '@/utils/format'

const MAX_BYTES = 2 << 20

const props = defineProps<{
  visible: boolean
  problemId: number
  testcase?: Testcase | null
}>()
const emit = defineEmits<{
  'update:visible': [value: boolean]
  saved: []
}>()

const toast = useToast()
const confirm = useConfirm()

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const input = ref('')
const output = ref('')
const score = ref(10)
const isSample = ref(false)

const isCreate = computed(() => !props.testcase)
const title = computed(() =>
  isCreate.value ? '在线创建测试点' : `编辑测试点 #${props.testcase?.sortOrder ?? ''}`,
)
const inputSize = computed(() => new TextEncoder().encode(input.value).length)
const outputSize = computed(() => new TextEncoder().encode(output.value).length)
const tooLarge = computed(() => inputSize.value > MAX_BYTES || outputSize.value > MAX_BYTES)
const initialInput = ref('')
const initialOutput = ref('')
const dirty = computed(
  () =>
    input.value !== initialInput.value ||
    output.value !== initialOutput.value ||
    (!isCreate.value ? false : score.value !== 10 || isSample.value),
)

function reset(values = { input: '', output: '', score: 10, isSample: false }) {
  input.value = values.input
  output.value = values.output
  initialInput.value = values.input
  initialOutput.value = values.output
  score.value = values.score
  isSample.value = values.isSample
  error.value = ''
}

async function load() {
  if (!props.visible) return
  error.value = ''
  if (!props.testcase) {
    reset()
    return
  }
  loading.value = true
  try {
    const data = await getTestcaseContent(props.problemId, props.testcase.id)
    reset({
      input: data.input,
      output: data.output,
      score: data.score,
      isSample: data.isSample,
    })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载测试数据失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.visible, props.testcase?.id],
  () => void load(),
  { immediate: true },
)

function requestClose() {
  if (!dirty.value) {
    emit('update:visible', false)
    return
  }
  confirm.require({
    header: '放弃修改',
    message: '测试数据还没有保存，确定关闭吗？',
    acceptLabel: '放弃',
    rejectLabel: '继续编辑',
    accept: () => emit('update:visible', false),
  })
}

function onVisibleChange(value: boolean) {
  if (!value) requestClose()
}

async function save() {
  if (tooLarge.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    if (props.testcase) {
      await updateTestcaseContent(props.problemId, props.testcase.id, {
        input: input.value,
        output: output.value,
      })
    } else {
      await createOnlineTestcase(props.problemId, {
        input: input.value,
        output: output.value,
        score: score.value,
        isSample: isSample.value,
      })
    }
    toast.add({
      severity: 'success',
      summary: isCreate.value ? '测试点已创建' : '测试点已保存',
      life: 3000,
    })
    initialInput.value = input.value
    initialOutput.value = output.value
    if (isCreate.value) {
      score.value = 10
      isSample.value = false
    }
    emit('saved')
    emit('update:visible', false)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '保存测试数据失败，请稍后再试'
  } finally {
    saving.value = false
  }
}

function download(kind: 'input' | 'output') {
  const text = kind === 'input' ? input.value : output.value
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `${kind}.txt`
  anchor.click()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <Dialog
    :visible="visible"
    :header="title"
    modal
    :draggable="false"
    :style="{ width: 'min(96vw, 1080px)' }"
    @update:visible="onVisibleChange"
  >
    <div class="tc-editor">
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <Message v-if="tooLarge" severity="warn" :closable="false">
        单个测试数据不能超过 2 MiB，请下载后使用文件替换。
      </Message>

      <div v-if="loading" class="tc-editor__loading">正在加载测试数据…</div>
      <div v-else class="tc-editor__grid">
        <section class="tc-editor__pane">
          <header>
            <span>输入数据</span>
            <span class="tc-editor__meta">{{ formatBytes(inputSize) }}</span>
            <Button
              label="下载"
              icon="pi pi-download"
              text
              size="small"
              :disabled="loading"
              @click="download('input')"
            />
          </header>
          <Textarea
            v-model="input"
            data-test="testcase-input"
            class="tc-editor__text"
            :class="{ 'tc-editor__text--invalid': inputSize > MAX_BYTES }"
            rows="16"
            spellcheck="false"
            aria-label="输入数据"
          />
        </section>

        <section class="tc-editor__pane">
          <header>
            <span>输出数据</span>
            <span class="tc-editor__meta">{{ formatBytes(outputSize) }}</span>
            <Button
              label="下载"
              icon="pi pi-download"
              text
              size="small"
              :disabled="loading"
              @click="download('output')"
            />
          </header>
          <Textarea
            v-model="output"
            data-test="testcase-output"
            class="tc-editor__text"
            :class="{ 'tc-editor__text--invalid': outputSize > MAX_BYTES }"
            rows="16"
            spellcheck="false"
            aria-label="输出数据"
          />
        </section>
      </div>

      <div v-if="isCreate" class="tc-editor__options">
        <label class="tc-editor__score">
          分数
          <InputNumber v-model="score" :min="0" :max="1000" size="small" />
        </label>
        <div class="tc-editor__sample">
          <ToggleSwitch v-model="isSample" input-id="online-testcase-sample" />
          <label for="online-testcase-sample">作为样例</label>
        </div>
      </div>
    </div>

    <template #footer>
      <Button label="取消" severity="secondary" text :disabled="saving" @click="requestClose" />
      <Button
        :label="isCreate ? '创建测试点' : '保存'"
        icon="pi pi-check"
        :loading="saving"
        :disabled="loading || tooLarge"
        @click="save"
      />
    </template>
  </Dialog>
</template>

<style scoped>
.tc-editor {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.tc-editor__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
}

.tc-editor__pane {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.tc-editor__pane header {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  font-size: 0.8125rem;
  font-weight: 600;
}

.tc-editor__meta {
  margin-left: auto;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  font-weight: 400;
}

.tc-editor__text {
  width: 100%;
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  line-height: 1.5;
}

.tc-editor__text--invalid {
  border-color: var(--kapoj-danger);
}

.tc-editor__loading {
  padding: var(--kapoj-space-5);
  color: var(--kapoj-text-muted);
  text-align: center;
}

.tc-editor__options {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-4);
}

.tc-editor__score,
.tc-editor__sample {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

@media (max-width: 760px) {
  .tc-editor__grid {
    grid-template-columns: 1fr;
  }
}
</style>
