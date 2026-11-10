<script setup lang="ts">
// The async wrapper forwards the markdown props to its loading component. Keep
// them off the placeholder DOM.
defineOptions({ inheritAttrs: false })
</script>

<template>
  <div
    class="markdown-view-skeleton"
    data-test="markdown-view-skeleton"
    aria-busy="true"
    aria-label="内容加载中"
  >
    <span v-for="n in 3" :key="n" class="markdown-view-skeleton__line" />
    <span class="markdown-view-skeleton__block" />
  </div>
</template>

<style scoped>
.markdown-view-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-height: 160px;
  padding: var(--kapoj-space-1) 0;
  overflow: hidden;
}

.markdown-view-skeleton__line,
.markdown-view-skeleton__block {
  background: linear-gradient(
    90deg,
    var(--kapoj-surface-raised) 0%,
    var(--kapoj-surface-hover) 50%,
    var(--kapoj-surface-raised) 100%
  );
  background-size: 200% 100%;
  border-radius: var(--kapoj-radius-xs);
  animation: markdown-view-skeleton-shimmer 1.4s ease-in-out infinite;
}

.markdown-view-skeleton__line {
  height: 10px;
}

.markdown-view-skeleton__line:nth-child(1) {
  width: 92%;
}

.markdown-view-skeleton__line:nth-child(2) {
  width: 78%;
}

.markdown-view-skeleton__line:nth-child(3) {
  width: 85%;
}

.markdown-view-skeleton__block {
  width: 100%;
  height: 72px;
  border-radius: var(--kapoj-radius-sm);
}

@keyframes markdown-view-skeleton-shimmer {
  0% {
    background-position: 200% 0;
  }

  100% {
    background-position: -200% 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .markdown-view-skeleton__line,
  .markdown-view-skeleton__block {
    animation: none;
  }
}
</style>
