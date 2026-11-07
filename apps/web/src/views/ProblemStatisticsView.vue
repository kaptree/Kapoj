<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import ProgressBar from 'primevue/progressbar'

import { ApiError } from '@/api/http'
import { getStatistics } from '@/api/problems'
import type { ProblemStats } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'

const route = useRoute()

const problemId = computed(() => {
  const id = route.params.id
  return typeof id === 'string' ? id : ''
})

const stats = ref<ProblemStats | null>(null)
const loading = ref(false)
const error = ref('')

const rate = computed(() => (stats.value ? `${(stats.value.acceptRate * 100).toFixed(1)}%` : '-'))
const acceptPercent = computed(() =>
  stats.value ? Math.min(100, Math.max(0, stats.value.acceptRate * 100)) : 0,
)

let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  stats.value = null
  try {
    const data = await getStatistics(problemId.value)
    if (seq !== latest) return
    stats.value = data
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载统计失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(() => route.params.id, load, { immediate: true })
</script>

<template>
  <div class="statistics-page">
    <AppPageHeader
      eyebrow="PROBLEM // STATISTICS"
      title="提交统计"
      :description="`题目 #${problemId}`"
    >
      <template #actions>
        <Button
          as="router-link"
          :to="`/problems/${problemId}`"
          label="返回题目"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
        <Button
          as="router-link"
          :to="`/submissions?problemId=${problemId}`"
          label="查看提交记录"
          icon="pi pi-list"
        />
      </template>
    </AppPageHeader>

    <AppSurface padding="lg">
      <AppLoadingState v-if="loading && !stats" label="正在加载统计" :rows="2" />
      <AppErrorState v-else-if="error && !stats" :description="error" @retry="load" />
      <AppEmptyState
        v-else-if="!stats"
        icon="pi pi-chart-bar"
        title="暂时无法获取统计"
        description="该题目暂时没有可用的提交统计。"
      />
      <div v-else class="stats">
        <dl class="stats-grid">
          <div>
            <dt>提交数</dt>
            <dd class="num">{{ stats.submitCount }}</dd>
          </div>
          <div>
            <dt>通过数</dt>
            <dd class="num">{{ stats.acceptCount }}</dd>
          </div>
          <div>
            <dt>通过率</dt>
            <dd class="num">{{ rate }}</dd>
          </div>
        </dl>
        <ProgressBar :value="acceptPercent" :show-value="false" class="stats-bar" />
      </div>
    </AppSurface>
  </div>
</template>

<style scoped>
.statistics-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
  margin: 0;
}

.stats-grid div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.stats-grid dt {
  color: var(--kapoj-text-muted);
  font-size: 10px;
  text-transform: uppercase;
}

.stats-grid dd {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.1rem;
  font-weight: 600;
}

.stats-bar {
  margin-top: var(--kapoj-space-4);
}

@media (max-width: 700px) {
  .stats-grid {
    gap: var(--kapoj-space-2);
  }
}
</style>
