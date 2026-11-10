<script setup lang="ts">
import { useId } from 'vue'
import { MdPreview } from 'md-editor-v3'

import '@/markdown'
import { sanitizeHtml } from '@/utils/sanitize'

withDefaults(
  defineProps<{
    source: string
    theme?: 'light' | 'dark'
  }>(),
  { theme: 'light' },
)

// Every preview on a page needs its own id.
const id = useId()

// Heading ids default to the heading text, which repeats across previews and can
// collide with ids elsewhere on the page. Prefix them per preview instead.
const headingId = ({ index }: { index: number }) => `md-${id}-${index}`
</script>

<template>
  <MdPreview
    :id="id"
    class="markdown-view"
    :model-value="source"
    :sanitize="sanitizeHtml"
    :md-heading-id="headingId"
    :theme="theme"
    language="zh-CN"
    no-mermaid
    no-echarts
    no-img-zoom-in
  />
</template>

<style scoped>
.markdown-view,
.markdown-view :deep(.md-editor-preview),
.markdown-view :deep(.md-editor-preview-wrapper) {
  background: transparent;
}

.markdown-view :deep(.md-editor-preview-wrapper) {
  padding: 0;
}

.markdown-view--dark :deep(.md-editor-preview) {
  color: var(--kapoj-text);
}

.markdown-view--dark :deep(.md-editor-preview h1),
.markdown-view--dark :deep(.md-editor-preview h2),
.markdown-view--dark :deep(.md-editor-preview h3),
.markdown-view--dark :deep(.md-editor-preview h4),
.markdown-view--dark :deep(.md-editor-preview h5),
.markdown-view--dark :deep(.md-editor-preview h6) {
  color: var(--kapoj-text);
}
</style>
