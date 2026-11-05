<script setup lang="ts">
import Button from 'primevue/button'

import { useTheme } from '@/theme/theme'

const { isDark, toggleTheme } = useTheme()
</script>

<template>
  <Button
    class="app-theme-toggle"
    :icon="isDark ? 'pi pi-sun' : 'pi pi-moon'"
    severity="secondary"
    text
    size="small"
    :aria-label="isDark ? '切换到亮色主题' : '切换到暗色主题'"
    :aria-pressed="isDark"
    :title="isDark ? '切换到亮色主题' : '切换到暗色主题'"
    @click="toggleTheme"
  />
</template>

<style scoped>
.app-theme-toggle {
  width: 2rem;
  height: 2rem;
  padding: 0;
  color: var(--kapoj-text-secondary);
  border-radius: var(--kapoj-radius-sm);
}

.app-theme-toggle:hover {
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.app-theme-toggle :deep(.p-button-icon) {
  font-size: 0.95rem;
}
</style>
