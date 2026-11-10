<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { createCodeEditor, type CodeEditorHandle } from '@/editor'
import { utf8Offset } from '@/utils/text'

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
let editor: CodeEditorHandle | undefined

onMounted(() => {
  if (!host.value) return
  editor = createCodeEditor(host.value, {
    value: props.modelValue,
    language: props.language,
    readonly: props.readonly,
    onChange: (value) => emit('update:modelValue', value),
  })
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
    editor?.setLanguage(language)
  },
)

onBeforeUnmount(() => {
  editor?.destroy()
  editor = undefined
})

/** UTF-8 byte offset of the cursor, or the end of the code when unavailable. */
function getCursorByteOffset(): number {
  if (!editor) return utf8Offset(props.modelValue, props.modelValue.length)
  return editor.getCursorByteOffset()
}

/** Inserts text at the current cursor and focuses the editor. */
function insertAtCursor(text: string) {
  editor?.insertAtCursor(text)
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
