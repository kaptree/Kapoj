<script setup lang="ts">
withDefaults(
  defineProps<{
    icon?: string
    title: string
    description?: string
  }>(),
  {
    icon: 'pi pi-inbox',
    description: '',
  },
)
</script>

<template>
  <div class="app-empty" role="status">
    <span class="app-empty__icon" aria-hidden="true">
      <i :class="icon" />
    </span>
    <h2 class="app-empty__title">{{ title }}</h2>
    <p v-if="description" class="app-empty__description">{{ description }}</p>
    <div v-if="$slots.actions" class="app-empty__actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<style scoped>
.app-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-7) var(--kapoj-space-4);
  text-align: center;
}

.app-empty__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  margin-bottom: var(--kapoj-space-1);
  color: var(--kapoj-primary);
  font-size: 1.25rem;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 28%, transparent);
  border-radius: var(--kapoj-radius-md);
  box-shadow: var(--kapoj-glow-primary);
}

.app-empty__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.app-empty__description {
  max-width: 48ch;
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.app-empty__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--kapoj-space-2);
  margin-top: var(--kapoj-space-2);
}

@media (max-width: 640px) {
  .app-empty {
    padding: var(--kapoj-space-5) var(--kapoj-space-3);
  }
}
</style>
