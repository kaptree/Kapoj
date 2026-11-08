<script setup lang="ts">
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import Button from 'primevue/button'
</script>

<template>
  <AppSurface padding="lg" class="result-surface">
    <p class="result-code">403</p>
    <AppEmptyState icon="pi pi-lock" title="没有权限" description="你的账号无法访问这个页面。">
      <template #actions>
        <Button as="router-link" to="/" label="返回首页" icon="pi pi-home" />
      </template>
    </AppEmptyState>
  </AppSurface>
</template>

<style scoped>
.result-surface {
  max-width: 560px;
  margin: 0 auto;
  text-align: center;
}

.result-code {
  margin: 0;
  font-family: var(--kapoj-font-mono);
  font-size: clamp(3rem, 8vw, 5rem);
  font-weight: 700;
  line-height: 1;
  color: var(--kapoj-text-secondary);
}

@media (max-width: 640px) {
  .result-code {
    font-size: clamp(2.5rem, 14vw, 3.5rem);
  }
}
</style>
