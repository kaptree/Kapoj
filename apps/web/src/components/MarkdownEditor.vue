<script setup lang="ts">
import { computed, useId, ref } from 'vue'
import { MdEditor, type ToolbarNames } from 'md-editor-v3'

import '@/markdown'
import { sanitizeHtml } from '@/utils/sanitize'

const model = defineModel<string>({ required: true })
const props = withDefaults(
  defineProps<{
    height?: string
    theme?: 'light' | 'dark'
    problemId?: number
    uploadImage?: (file: File) => Promise<string>
  }>(),
  { height: '360px', theme: 'light', problemId: undefined, uploadImage: undefined },
)

const id = useId()
const imageError = ref('')

// Mermaid, prettier and fullscreen each pull in extra libraries from a CDN;
// none of them is needed to write a problem statement. The image button is
// only available after the problem has an ID because uploads are scoped to it.
const toolbarsExclude = computed<ToolbarNames[]>(() => [
  ...(props.problemId ? [] : (['image'] as ToolbarNames[])),
  'mermaid',
  'prettier',
  'github',
  'fullscreen',
  'pageFullscreen',
  'htmlPreview',
  'catalog',
  'save',
])

async function uploadImages(files: File[], callback: (urls: string[]) => void) {
  imageError.value = ''
  if (!props.problemId || !props.uploadImage) {
    imageError.value = '请先保存题目，再上传图片'
    callback([])
    return
  }
  const urls: string[] = []
  for (const file of files) {
    try {
      urls.push(await props.uploadImage(file))
    } catch (e) {
      imageError.value = e instanceof Error ? e.message : '图片上传失败，请稍后再试'
    }
  }
  callback(urls)
}
</script>

<template>
  <p v-if="imageError" class="markdown-editor__error" role="alert">{{ imageError }}</p>
  <MdEditor
    :id="id"
    v-model="model"
    :sanitize="sanitizeHtml"
    :toolbars-exclude="toolbarsExclude"
    :style="{ height }"
    :theme="theme"
    :no-upload-img="!problemId"
    :on-upload-img="uploadImages"
    language="zh-CN"
    no-mermaid
    no-echarts
    no-prettier
    no-img-zoom-in
  />
</template>

<style scoped>
.markdown-editor__error {
  margin: 0 0 var(--kapoj-space-2);
  color: var(--kapoj-danger);
  font-size: 0.8125rem;
}
</style>
