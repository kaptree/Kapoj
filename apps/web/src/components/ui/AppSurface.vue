<script setup lang="ts">
withDefaults(
  defineProps<{
    as?: string
    padding?: 'none' | 'sm' | 'md' | 'lg'
    tone?: 'default' | 'raised' | 'accent'
    interactive?: boolean
  }>(),
  {
    as: 'section',
    padding: 'md',
    tone: 'default',
    interactive: false,
  },
)
</script>

<template>
  <component
    :is="as"
    class="app-surface"
    :class="[
      `app-surface--pad-${padding}`,
      `app-surface--${tone}`,
      { 'app-surface--interactive': interactive },
    ]"
  >
    <slot />
  </component>
</template>

<style scoped>
.app-surface {
  min-width: 0;
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
  box-shadow: var(--kapoj-shadow-sm);
  transition:
    border-color var(--kapoj-transition),
    box-shadow var(--kapoj-transition),
    transform var(--kapoj-transition);
}

.app-surface--raised {
  background: var(--kapoj-bg-elevated);
  box-shadow: var(--kapoj-shadow-md);
}

.app-surface--accent {
  border-color: color-mix(in srgb, var(--kapoj-primary) 45%, var(--kapoj-border));
  box-shadow: var(--kapoj-glow-primary);
}

.app-surface--interactive:hover {
  border-color: var(--kapoj-border-strong);
  transform: translateY(-1px);
}

.app-surface--pad-sm {
  padding: var(--kapoj-space-3);
}

.app-surface--pad-md {
  padding: var(--kapoj-space-4);
}

.app-surface--pad-lg {
  padding: var(--kapoj-space-5);
}

@media (max-width: 640px) {
  .app-surface--pad-lg {
    padding: var(--kapoj-space-4);
  }
}

@media (prefers-reduced-motion: reduce) {
  .app-surface {
    transition: none;
  }

  .app-surface--interactive:hover {
    transform: none;
  }
}
</style>
