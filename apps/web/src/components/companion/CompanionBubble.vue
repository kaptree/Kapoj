<script setup lang="ts">
withDefaults(
  defineProps<{
    text: string
    tone?: 'tsun' | 'neutral'
  }>(),
  { tone: 'tsun' },
)
</script>

<template>
  <p class="companion-bubble" :class="`companion-bubble--${tone}`" role="status" aria-live="polite">
    {{ text }}
  </p>
</template>

<style scoped>
.companion-bubble {
  position: relative;
  max-width: 34ch;
  padding: 8px 14px;
  margin: 0;
  font-size: 0.875rem;
  line-height: 1.55;
  color: var(--kapoj-text);
  background: color-mix(in srgb, var(--kapoj-silver) 9%, var(--kapoj-surface));
  border: 1px solid var(--kapoj-manga-line);
  border-radius: 12px;
}

.companion-bubble--tsun {
  border-color: color-mix(in srgb, var(--kapoj-kapo) 40%, var(--kapoj-manga-line));
}

.companion-bubble::after {
  position: absolute;
  bottom: -6px;
  left: 18px;
  width: 10px;
  height: 10px;
  content: '';
  background: inherit;
  border-right: 1px solid var(--kapoj-manga-line);
  border-bottom: 1px solid var(--kapoj-manga-line);
  transform: rotate(45deg);
}

.companion-bubble--tsun::after {
  border-color: color-mix(in srgb, var(--kapoj-kapo) 40%, var(--kapoj-manga-line));
}
</style>
