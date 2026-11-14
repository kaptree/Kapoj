<script setup lang="ts">
import { ref, watch } from 'vue'

import { listProblemAttachments } from '@/api/problems'
import type { ProblemAttachment } from '@/api/types'
import AppSurface from '@/components/ui/AppSurface.vue'
import { resolveAssetUrl } from '@/utils/asset'
import { formatBytes } from '@/utils/format'

const props = defineProps<{ problemId: number }>()
const items = ref<ProblemAttachment[]>([])

let latest = 0

async function load() {
  const seq = ++latest
  try {
    const list = await listProblemAttachments(props.problemId)
    if (seq === latest) items.value = list
  } catch {
    if (seq === latest) items.value = []
  }
}

watch(
  () => props.problemId,
  () => void load(),
  { immediate: true },
)
</script>

<template>
  <AppSurface v-if="items.length" padding="md" class="attachment-card">
    <div class="attachment-card__head">
      <span class="attachment-card__icon" aria-hidden="true"><i class="pi pi-paperclip" /></span>
      <span class="attachment-card__title">题目附件</span>
    </div>
    <ul class="attachment-card__list">
      <li v-for="item in items" :key="item.id">
        <a :href="resolveAssetUrl(item.url)" download>{{ item.filename }}</a>
        <span>{{ formatBytes(item.size) }}</span>
      </li>
    </ul>
  </AppSurface>
</template>

<style scoped>
.attachment-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
}

.attachment-card__head {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.attachment-card__icon {
  color: var(--kapoj-primary);
}

.attachment-card__title {
  color: var(--kapoj-text);
  font-weight: 600;
}

.attachment-card__list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 0;
  margin: 0;
  list-style: none;
}

.attachment-card__list li {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  min-width: 0;
  font-size: 0.8125rem;
}

.attachment-card__list a {
  overflow: hidden;
  color: var(--kapoj-text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.attachment-card__list span {
  flex: none;
  color: var(--kapoj-text-muted);
  font-size: 11px;
}
</style>
