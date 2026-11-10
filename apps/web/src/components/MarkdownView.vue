<script setup lang="ts">
import { defineAsyncComponent } from 'vue'

import MarkdownViewSkeleton from './MarkdownViewSkeleton.vue'

// Keep the markdown renderer and its math/highlight dependencies out of every
// route chunk that shows a statement. The theme class is applied here so it
// reaches the rendered preview root through attribute fallthrough.
const Content = defineAsyncComponent({
  loader: () => import('./MarkdownViewContent.vue'),
  loadingComponent: MarkdownViewSkeleton,
  delay: 0,
})

withDefaults(defineProps<{ source: string; theme?: 'light' | 'dark' }>(), {
  theme: 'light',
})
</script>

<template>
  <Content
    :source="source"
    :theme="theme"
    :class="theme === 'dark' ? 'markdown-view--dark' : 'markdown-view--light'"
  />
</template>
