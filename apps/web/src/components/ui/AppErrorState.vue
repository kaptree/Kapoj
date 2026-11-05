<script setup lang="ts">
import Button from 'primevue/button'

withDefaults(
  defineProps<{
    title?: string
    description: string
    retryLabel?: string
  }>(),
  {
    title: '加载失败',
    retryLabel: '重试',
  },
)

const emit = defineEmits<{ retry: [] }>()
</script>

<template>
  <div class="app-error" role="alert">
    <span class="app-error__icon" aria-hidden="true">
      <i class="pi pi-exclamation-triangle" />
    </span>
    <div class="app-error__body">
      <h2 class="app-error__title">{{ title }}</h2>
      <p class="app-error__description">{{ description }}</p>
    </div>
    <Button
      :label="retryLabel"
      icon="pi pi-refresh"
      severity="secondary"
      outlined
      @click="emit('retry')"
    />
  </div>
</template>

<style scoped>
.app-error {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-4);
  background: color-mix(in srgb, var(--kapoj-danger) 8%, var(--kapoj-surface));
  border: 1px solid color-mix(in srgb, var(--kapoj-danger) 28%, transparent);
  border-radius: var(--kapoj-radius-md);
}

.app-error__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  color: var(--kapoj-danger);
  font-size: 1.1rem;
  background: color-mix(in srgb, var(--kapoj-danger) 12%, transparent);
  border-radius: var(--kapoj-radius-sm);
}

.app-error__body {
  flex: 1 1 220px;
  min-width: 0;
}

.app-error__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.95rem;
  font-weight: 600;
}

.app-error__description {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

@media (max-width: 640px) {
  .app-error {
    align-items: flex-start;
  }

  .app-error :deep(.p-button) {
    width: 100%;
  }
}
</style>
