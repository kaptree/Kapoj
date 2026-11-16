<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  bond: number
  level: number
  levelName: string
  nextThreshold: number
  dailyGain: number
  dailyCap: number
}>()

const summary = computed(
  () => `Lv.${props.level} ${props.levelName} ${props.bond}/${props.nextThreshold}`,
)

/** A missing or zero threshold keeps the track empty instead of dividing by it. */
const fillPercent = computed(() => {
  if (!Number.isFinite(props.nextThreshold) || props.nextThreshold <= 0) return 0
  const ratio = (props.bond / props.nextThreshold) * 100
  return Math.min(100, Math.max(0, ratio))
})
</script>

<template>
  <div class="companion-bond" data-test="companion-bond">
    <p class="companion-bond__text">{{ summary }}</p>
    <div
      class="companion-bond__track"
      role="progressbar"
      :aria-label="`与${levelName}的羁绊`"
      aria-valuemin="0"
      :aria-valuemax="nextThreshold"
      :aria-valuenow="bond"
    >
      <span class="companion-bond__fill" :style="{ width: `${fillPercent}%` }" />
    </div>
    <p class="companion-bond__daily">今日 {{ dailyGain }}/{{ dailyCap }}</p>
  </div>
</template>

<style scoped>
.companion-bond {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.companion-bond__text,
.companion-bond__daily {
  margin: 0;
  font-size: 0.78rem;
  line-height: 1.4;
}

.companion-bond__text {
  color: var(--kapoj-text);
  font-weight: 600;
}

.companion-bond__daily {
  color: var(--kapoj-text-secondary);
}

.companion-bond__track {
  height: 6px;
  overflow: hidden;
  background: color-mix(in srgb, var(--kapoj-kapo) 12%, var(--kapoj-border));
  border-radius: 999px;
}

.companion-bond__fill {
  display: block;
  height: 100%;
  background: var(--kapoj-kapo);
  border-radius: inherit;
  transition: width 200ms ease;
}

@media (prefers-reduced-motion: reduce) {
  .companion-bond__fill {
    transition: none;
  }
}
</style>
