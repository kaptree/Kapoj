<script setup lang="ts">
import { computed } from 'vue'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'
import hljs from 'highlight.js/lib/core'
import c from 'highlight.js/lib/languages/c'
import cpp from 'highlight.js/lib/languages/cpp'
import go from 'highlight.js/lib/languages/go'
import java from 'highlight.js/lib/languages/java'
import python from 'highlight.js/lib/languages/python'
import 'highlight.js/styles/atom-one-dark.css'

hljs.registerLanguage('c', c)
hljs.registerLanguage('cpp', cpp)
hljs.registerLanguage('go', go)
hljs.registerLanguage('java', java)
hljs.registerLanguage('python', python)

const props = defineProps<{
  code: string
  language: string
}>()

const toast = useToast()

const language = computed(() => {
  switch (props.language) {
    case 'c':
      return 'c'
    case 'cpp':
      return 'cpp'
    case 'python3':
      return 'python'
    case 'java':
      return 'java'
    case 'go':
      return 'go'
    default:
      return ''
  }
})

const highlighted = computed(() => {
  if (!props.code) return ''
  if (!language.value) return escapeHtml(props.code)
  try {
    return hljs.highlight(props.code, { language: language.value }).value
  } catch {
    return escapeHtml(props.code)
  }
})

const lineNumbers = computed(() => {
  const count = Math.max(1, props.code.split('\n').length)
  return Array.from({ length: count }, (_, index) => index + 1).join('\n')
})

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

async function copy() {
  try {
    await navigator.clipboard.writeText(props.code)
    toast.add({ severity: 'success', summary: '代码已复制', life: 3000 })
  } catch {
    toast.add({ severity: 'warn', summary: '复制失败，请手动选择代码', life: 3000 })
  }
}
</script>

<template>
  <div class="code-viewer">
    <div class="code-viewer__toolbar">
      <span class="code-viewer__lang">{{ language || '代码' }}</span>
      <Button
        label="复制"
        icon="pi pi-copy"
        text
        size="small"
        data-test="code-copy"
        @click="copy"
      />
    </div>
    <div class="code-viewer__body" tabindex="0">
      <pre class="code-viewer__gutter" aria-hidden="true">{{ lineNumbers }}</pre>
      <!-- eslint-disable-next-line vue/no-v-html -- highlighted() escapes unknown languages and only emits highlight.js output -->
      <pre class="code-viewer__code"><code class="hljs" v-html="highlighted" /></pre>
    </div>
  </div>
</template>

<style scoped>
.code-viewer {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #282c34;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.code-viewer__toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 32px;
  padding: 0 var(--kapoj-space-2);
  color: #abb2bf;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.code-viewer__lang {
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  text-transform: uppercase;
}

.code-viewer__body {
  display: flex;
  max-height: 460px;
  overflow: auto;
}

.code-viewer__gutter,
.code-viewer__code {
  margin: 0;
  padding: var(--kapoj-space-2) 0;
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  line-height: 1.6;
}

.code-viewer__gutter {
  flex: none;
  padding-right: var(--kapoj-space-2);
  padding-left: var(--kapoj-space-2);
  color: #5c6370;
  text-align: right;
  user-select: none;
  border-right: 1px solid rgba(255, 255, 255, 0.06);
}

.code-viewer__code {
  flex: 1;
  padding-right: var(--kapoj-space-3);
  padding-left: var(--kapoj-space-2);
  color: #abb2bf;
  white-space: pre;
}

.code-viewer__code code {
  display: block;
  padding: 0;
  background: transparent;
}
</style>
