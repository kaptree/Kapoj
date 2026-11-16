<script setup lang="ts">
withDefaults(
  defineProps<{
    index?: string
    label: string
    kana?: string
  }>(),
  { index: '', kana: '' },
)
</script>

<template>
  <p class="manga-label">
    <span v-if="index" class="manga-label__index">{{ index }}</span>
    <span class="manga-label__label">{{ label }}</span>
    <span v-if="kana" class="manga-label__kana" aria-hidden="true">{{ kana }}</span>
  </p>
</template>

<style scoped>
.manga-label {
  display: flex;
  gap: 10px;
  align-items: baseline;
  padding: 2px 0 6px 10px;
  margin: 0;
  border-bottom: 1px solid var(--kapoj-manga-line);
  border-left: 3px solid var(--kapoj-kapo);
}

.manga-label__index {
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  color: var(--kapoj-kapo-text);
  text-transform: uppercase;
}

.manga-label__label {
  font-size: 0.95rem;
  font-weight: 600;
  line-height: 1.4;
  color: var(--kapoj-text);
}

.manga-label__kana {
  font-size: 11px;
  letter-spacing: 0.14em;
  color: var(--kapoj-text-muted);
}
</style>
