<script setup lang="ts">
defineProps<{
  eyebrow?: string
  title: string
  description?: string
}>()
</script>

<template>
  <header class="app-page-header">
    <div class="app-page-header__main">
      <p v-if="eyebrow" class="app-page-header__eyebrow">{{ eyebrow }}</p>
      <h1 class="app-page-header__title">{{ title }}</h1>
      <p v-if="description" class="app-page-header__description">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="app-page-header__actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<style scoped>
.app-page-header {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--kapoj-space-4);
}

.app-page-header__main {
  min-width: 0;
}

.app-page-header__eyebrow {
  margin: 0 0 var(--kapoj-space-1);
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.app-page-header__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: clamp(1.35rem, 1.1rem + 1vw, 1.9rem);
  line-height: 1.2;
  letter-spacing: -0.02em;
}

.app-page-header__description {
  max-width: 70ch;
  margin: var(--kapoj-space-2) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.app-page-header__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

@media (max-width: 640px) {
  .app-page-header__actions {
    width: 100%;
  }

  .app-page-header__actions :deep(.p-button) {
    flex: 1 1 auto;
  }
}
</style>
