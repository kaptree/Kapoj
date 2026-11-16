<script setup lang="ts">
import type { CompanionExpression } from '../../content/companion/types'
import CompanionAvatar from './CompanionAvatar.vue'

withDefaults(
  defineProps<{
    line: string
    expression?: CompanionExpression
    dismissible?: boolean
  }>(),
  { expression: 'smug', dismissible: true },
)

const emit = defineEmits<{ dismiss: [] }>()
</script>

<template>
  <div class="companion-note">
    <CompanionAvatar pose="chibi" :expression="expression" size="xs" />
    <p class="companion-note__line">{{ line }}</p>
    <button
      v-if="dismissible"
      class="companion-note__close"
      type="button"
      aria-label="关闭伙伴提示"
      @click="emit('dismiss')"
    >
      <span aria-hidden="true">×</span>
    </button>
  </div>
</template>

<style scoped>
.companion-note {
  display: flex;
  gap: 10px;
  align-items: center;
  box-sizing: border-box;
  min-height: 44px;
  padding: 6px 10px;
  background: var(--kapoj-kapo-soft);
  border: 1px solid var(--kapoj-manga-line);
  border-left: 3px solid var(--kapoj-kapo);
  border-radius: var(--kapoj-radius-sm);
}

.companion-note :deep(.companion-avatar) {
  width: 28px;
}

.companion-note__line {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: 0.85rem;
  line-height: 1.5;
  color: var(--kapoj-text);
}

.companion-note__close {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  font-size: 1rem;
  line-height: 1;
  color: var(--kapoj-text-muted);
  cursor: pointer;
  background: transparent;
  border: 0;
  border-radius: 4px;
}

.companion-note__close:hover {
  color: var(--kapoj-text);
  background: color-mix(in srgb, var(--kapoj-manga-line) 40%, transparent);
}

.companion-note__close:focus-visible {
  outline: 2px solid var(--kapoj-kapo);
  outline-offset: 1px;
}
</style>
