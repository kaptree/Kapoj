<script setup lang="ts">
import { getActivePinia } from 'pinia'
import { computed } from 'vue'
import CompanionAvatar from '@/components/companion/CompanionAvatar.vue'
import type { CompanionExpression } from '@/content/companion/types'
import { useCompanionStore } from '@/stores/companion'

const props = withDefaults(
  defineProps<{
    icon?: string
    title: string
    description?: string
    mascot?: CompanionExpression | ''
    companionLine?: string
  }>(),
  {
    icon: 'pi pi-inbox',
    description: '',
    mascot: '',
    companionLine: '',
  },
)

// Isolated mounts (tests, stories) may not install Pinia; the mascot falls back
// to visible there instead of crashing the whole state component.
const companion = getActivePinia() ? useCompanionStore() : null
companion?.load()

const showMascot = computed(() => Boolean(props.mascot) && (companion?.enabled ?? true))
</script>

<template>
  <div class="app-empty" role="status">
    <CompanionAvatar v-if="showMascot" pose="bust" :expression="mascot || 'neutral'" size="md" />
    <span v-else class="app-empty__icon" aria-hidden="true">
      <i :class="icon" />
    </span>
    <h2 class="app-empty__title">{{ title }}</h2>
    <p v-if="description" class="app-empty__description">{{ description }}</p>
    <p v-if="showMascot && companionLine" class="app-empty__companion">{{ companionLine }}</p>
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

.app-empty__companion {
  max-width: 32ch;
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  line-height: 1.5;
  text-wrap: balance;
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
