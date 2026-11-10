<script setup lang="ts">
// The async wrapper forwards the editor's props (modelValue, language, ...) to
// its loading component. Keep them off the placeholder DOM.
defineOptions({ inheritAttrs: false })

withDefaults(defineProps<{ height?: string }>(), { height: '100%' })
</script>

<template>
  <div
    class="code-editor-skeleton"
    data-test="code-editor-skeleton"
    :style="{ height }"
    aria-busy="true"
    aria-label="代码编辑器加载中"
  >
    <span v-for="n in 6" :key="n" class="code-editor-skeleton__line" />
  </div>
</template>

<style scoped>
.code-editor-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-3);
  overflow: hidden;
  /* Matches the one-dark background used by the CodeMirror editor. */
  background: #282c34;
  border-radius: var(--kapoj-radius-sm);
}

.code-editor-skeleton__line {
  height: 10px;
  background: linear-gradient(90deg, #2c313a 0%, #3e4451 50%, #2c313a 100%);
  background-size: 200% 100%;
  border-radius: var(--kapoj-radius-xs);
  animation: code-editor-skeleton-shimmer 1.4s ease-in-out infinite;
}

.code-editor-skeleton__line:nth-child(3n + 1) {
  width: 72%;
}

.code-editor-skeleton__line:nth-child(3n + 2) {
  width: 88%;
}

.code-editor-skeleton__line:nth-child(3n + 3) {
  width: 56%;
}

@keyframes code-editor-skeleton-shimmer {
  0% {
    background-position: 200% 0;
  }

  100% {
    background-position: -200% 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .code-editor-skeleton__line {
    animation: none;
  }
}
</style>
