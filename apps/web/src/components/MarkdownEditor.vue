<script setup lang="ts">
import { useId } from 'vue'
import { MdEditor, type ToolbarNames } from 'md-editor-v3'

import '@/markdown'
import { sanitizeHtml } from '@/utils/sanitize'

const model = defineModel<string>({ required: true })
withDefaults(defineProps<{ height?: string; theme?: 'light' | 'dark' }>(), {
  height: '360px',
  theme: 'light',
})

const id = useId()

// Image upload, mermaid, prettier and fullscreen each pull in extra libraries
// from a CDN; none of them is needed to write a problem statement.
const toolbarsExclude: ToolbarNames[] = [
  'image',
  'mermaid',
  'prettier',
  'github',
  'fullscreen',
  'pageFullscreen',
  'htmlPreview',
  'catalog',
  'save',
]
</script>

<template>
  <MdEditor
    :id="id"
    v-model="model"
    :sanitize="sanitizeHtml"
    :toolbars-exclude="toolbarsExclude"
    :style="{ height }"
    :theme="theme"
    language="zh-CN"
    no-mermaid
    no-echarts
    no-prettier
    no-upload-img
    no-img-zoom-in
  />
</template>
