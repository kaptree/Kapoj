<script setup lang="ts">
import { computed } from 'vue'

import { difficultyBand, difficultyText } from '@/utils/difficulty'

const props = defineProps<{ difficulty: number; showLabel?: boolean }>()

const band = computed(() => difficultyBand(props.difficulty))
const text = computed(() => {
  if (props.showLabel) return difficultyText(props.difficulty)
  return band.value === 0 ? '未知' : String(props.difficulty)
})
</script>

<template>
  <span class="app-difficulty-tag" :class="`app-difficulty-tag--${band === 0 ? 'unknown' : band}`">
    {{ text }}
  </span>
</template>

<style scoped>
.app-difficulty-tag {
  --difficulty-color: var(--kapoj-difficulty-unknown);

  display: inline-flex;
  align-items: center;
  padding: 1px var(--kapoj-space-2);
  color: var(--difficulty-color);
  font-size: 0.75rem;
  font-weight: 600;
  line-height: 1.5;
  white-space: nowrap;
  background: color-mix(in srgb, var(--difficulty-color) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--difficulty-color) 32%, transparent);
  border-radius: var(--kapoj-radius-xs);
}

.app-difficulty-tag--1 {
  --difficulty-color: var(--kapoj-difficulty-1);
}

.app-difficulty-tag--2 {
  --difficulty-color: var(--kapoj-difficulty-2);
}

.app-difficulty-tag--3 {
  --difficulty-color: var(--kapoj-difficulty-3);
}

.app-difficulty-tag--4 {
  --difficulty-color: var(--kapoj-difficulty-4);
}

.app-difficulty-tag--5 {
  --difficulty-color: var(--kapoj-difficulty-5);
}

.app-difficulty-tag--unknown {
  --difficulty-color: var(--kapoj-difficulty-unknown);
}
</style>
