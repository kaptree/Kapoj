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
