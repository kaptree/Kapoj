<script setup lang="ts">
withDefaults(
  defineProps<{
    label?: string
    rows?: number
  }>(),
  {
    label: '加载中',
    rows: 3,
  },
)
</script>

<template>
  <div class="app-loading" role="status" aria-live="polite">
    <span class="app-loading__spinner" aria-hidden="true" />
    <span class="app-loading__label">{{ label }}</span>
    <div class="app-loading__rows" aria-hidden="true">
      <span v-for="row in rows" :key="row" class="app-loading__row" />
    </div>
  </div>
</template>

<style scoped>
.app-loading {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-4);
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.app-loading__spinner {
  width: 20px;
  height: 20px;
  border: 2px solid var(--kapoj-border);
  border-top-color: var(--kapoj-primary);
  border-radius: 50%;
  animation: app-loading-spin 0.8s linear infinite;
}

.app-loading__label {
  font-weight: 500;
}

.app-loading__rows {
  display: grid;
  flex: 1 1 100%;
  gap: var(--kapoj-space-2);
  margin-top: var(--kapoj-space-2);
}

.app-loading__row {
  height: 10px;
  background: linear-gradient(
    90deg,
    var(--kapoj-surface-raised) 0%,
    var(--kapoj-surface-hover) 50%,
    var(--kapoj-surface-raised) 100%
  );
  background-size: 200% 100%;
  border-radius: var(--kapoj-radius-xs);
  animation: app-loading-shimmer 1.4s ease-in-out infinite;
}

.app-loading__row:nth-child(3n + 1) {
  width: 92%;
}

.app-loading__row:nth-child(3n + 2) {
  width: 76%;
}

.app-loading__row:nth-child(3n + 3) {
  width: 84%;
}

@keyframes app-loading-spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes app-loading-shimmer {
  0% {
    background-position: 200% 0;
  }

  100% {
    background-position: -200% 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .app-loading__spinner,
  .app-loading__row {
    animation: none;
  }
}
</style>
