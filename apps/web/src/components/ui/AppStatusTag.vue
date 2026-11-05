<script setup lang="ts">
import { computed } from 'vue'

import type { SubmissionStatus } from '@/api/types'
import { statusLabel } from '@/utils/submission'

const props = defineProps<{
  status: SubmissionStatus
  label?: string
}>()

const tones: Record<SubmissionStatus, string> = {
  pending: 'neutral',
  judging: 'judging',
  accepted: 'accepted',
  wrong_answer: 'wrong-answer',
  time_limit_exceeded: 'tle',
  memory_limit_exceeded: 'mle',
  runtime_error: 'runtime-error',
  compile_error: 'compile-error',
  system_error: 'system-error',
}

const tone = computed(() => tones[props.status] ?? 'neutral')
const text = computed(() => props.label ?? statusLabel(props.status))
</script>

<template>
  <span class="app-status-tag" :class="`app-status-tag--${tone}`" :data-status="status">
    <span class="app-status-tag__dot" aria-hidden="true" />
    <span>{{ text }}</span>
  </span>
</template>

<style scoped>
.app-status-tag {
  --status-color: var(--kapoj-status-ce);

  display: inline-flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding: 1px var(--kapoj-space-2);
  color: var(--status-color);
  font-size: 0.75rem;
  font-weight: 600;
  line-height: 1.5;
  white-space: nowrap;
  background: color-mix(in srgb, var(--status-color) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--status-color) 32%, transparent);
  border-radius: var(--kapoj-radius-xs);
}

.app-status-tag__dot {
  width: 6px;
  height: 6px;
  background: var(--status-color);
  border-radius: 50%;
  box-shadow: 0 0 6px var(--status-color);
}

.app-status-tag--accepted {
  --status-color: var(--kapoj-status-ac);
}

.app-status-tag--wrong-answer {
  --status-color: var(--kapoj-status-wa);
}

.app-status-tag--tle {
  --status-color: var(--kapoj-status-tle);
}

.app-status-tag--mle {
  --status-color: var(--kapoj-status-mle);
}

.app-status-tag--runtime-error {
  --status-color: var(--kapoj-status-re);
}

.app-status-tag--compile-error {
  --status-color: var(--kapoj-status-ce);
}

.app-status-tag--judging {
  --status-color: var(--kapoj-status-judging);
}

.app-status-tag--judging .app-status-tag__dot {
  animation: app-status-pulse 1.4s ease-in-out infinite;
}

.app-status-tag--system-error {
  --status-color: var(--kapoj-status-se);
}

@keyframes app-status-pulse {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }

  50% {
    opacity: 0.45;
    transform: scale(0.8);
  }
}

@media (prefers-reduced-motion: reduce) {
  .app-status-tag--judging .app-status-tag__dot {
    animation: none;
  }
}
</style>
