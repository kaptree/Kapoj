<script setup lang="ts">
import { useId } from 'vue'
import { MdPreview } from 'md-editor-v3'

import '@/markdown'
import { sanitizeHtml } from '@/utils/sanitize'

defineProps<{ source: string }>()

// Every preview on a page needs its own id.
const id = useId()

// Heading ids default to the heading text, which repeats across previews and can
// collide with ids elsewhere on the page. Prefix them per preview instead.
const headingId = ({ index }: { index: number }) => `md-${id}-${index}`
</script>

<template>
  <MdPreview
    :id="id"
    :model-value="source"
    :sanitize="sanitizeHtml"
    :md-heading-id="headingId"
    language="zh-CN"
    no-mermaid
    no-echarts
    no-img-zoom-in
  />
</template>
