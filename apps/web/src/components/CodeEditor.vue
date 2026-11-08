<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { monaco } from '@/monaco'

const props = withDefaults(
  defineProps<{
    modelValue: string
    language?: string
    height?: string
    readonly?: boolean
  }>(),
  { language: 'cpp', height: '420px', readonly: false },
)
const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()

const host = ref<HTMLElement>()
let editor: ReturnType<typeof monaco.editor.create> | undefined

onMounted(() => {
  if (!host.value) return
  editor = monaco.editor.create(host.value, {
    value: props.modelValue,
    language: props.language,
    theme: 'vs-dark',
    readOnly: props.readonly,
    automaticLayout: true,
    minimap: { enabled: false },
    fontSize: 14,
    scrollBeyondLastLine: false,
    tabSize: 4,
  })
  editor.onDidChangeModelContent(() => emit('update:modelValue', editor!.getValue()))
})

watch(
  () => props.modelValue,
  (value) => {
    if (editor && editor.getValue() !== value) editor.setValue(value)
  },
)

watch(
  () => props.language,
  (language) => {
    const model = editor?.getModel()
    if (model) monaco.editor.setModelLanguage(model, language)
  },
)

onBeforeUnmount(() => {
  editor?.dispose()
  editor = undefined
})

/** UTF-8 byte offset of the cursor, or the end of the code when unavailable. */
function getCursorByteOffset(): number {
  if (!editor) return new TextEncoder().encode(props.modelValue).length
  const position = editor.getPosition()
  const model = editor.getModel()
  if (!position || !model) return new TextEncoder().encode(props.modelValue).length
  const offset = model.getOffsetAt(position)
  return new TextEncoder().encode(model.getValue().slice(0, offset)).length
}

/** Inserts text at the current cursor and focuses the editor. */
function insertAtCursor(text: string) {
  if (!editor) return
  const position = editor.getPosition()
  const model = editor.getModel()
  if (!position || !model) return
  editor.executeEdits('ai-insert', [
    {
      range: {
        startLineNumber: position.lineNumber,
        startColumn: position.column,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      },
      text,
    },
  ])
  editor.focus()
}

defineExpose({ getCursorByteOffset, insertAtCursor })
</script>

<template>
  <div ref="host" class="code-editor" :style="{ height }" />
</template>

<style scoped>
.code-editor {
  width: 100%;
  overflow: hidden;
  border-radius: 4px;
}
</style>
