<script setup lang="ts">
import { computed } from 'vue'
import ProgressBar from 'primevue/progressbar'

import type { LearningPathItem, LearningPathNodeItem } from '@/api/types'
import { flattenLearningNodes, nodeProgressPercent } from '@/utils/learning'

const props = withDefaults(
  defineProps<{
    path: LearningPathItem
    nodes: LearningPathNodeItem[]
    showSolved: boolean
    compact?: boolean
  }>(),
  { compact: false },
)

const authorLabel = computed(() => props.path.author.nickname || props.path.author.username)

const problemStats = computed(() => {
  const seen = new Map<number, boolean>()
  for (const node of flattenLearningNodes(props.nodes)) {
    for (const problem of node.problems) {
      if (problem.solved) seen.set(problem.problemId, true)
      else if (!seen.has(problem.problemId)) seen.set(problem.problemId, false)
    }
  }
  let solved = 0
  for (const value of seen.values()) {
    if (value) solved += 1
  }
  return { total: seen.size, solved }
})

const problemLabel = computed(() => {
  if (!props.showSolved) return `${problemStats.value.total} 题`
  return `已完成 ${problemStats.value.solved} / ${problemStats.value.total} 题`
})

const nodePercent = computed(() =>
  nodeProgressPercent(props.path.myCompletedCount, props.path.nodeCount),
)

const nodeProgressLabel = computed(
  () => `已完成 ${props.path.myCompletedCount} / ${props.path.nodeCount} 节点`,
)

const ringLabel = computed(() => `${nodeProgressLabel.value}，${nodePercent.value}%`)
</script>

<template>
  <section class="path-summary" :class="{ 'path-summary--compact': compact }" aria-label="路线摘要">
    <div v-if="!compact" class="summary-meta">
      <span class="summary-author">作者 {{ authorLabel }}</span>
      <span class="summary-node-count">{{ path.nodeCount }} 个节点</span>
    </div>

    <span class="summary-problem-progress">{{ problemLabel }}</span>

    <div class="summary-completion">
      <template v-if="compact">
        <ProgressBar
          :value="nodePercent"
          :show-value="false"
          class="summary-progress-bar"
          :aria-label="nodeProgressLabel"
        />
        <span class="summary-progress-label">{{ nodeProgressLabel }}</span>
      </template>
      <template v-else>
        <div
          class="summary-ring"
          :class="{ 'summary-ring--empty': path.nodeCount === 0 }"
          :style="{ '--progress': `${nodePercent}%` }"
          role="img"
          :aria-label="ringLabel"
        >
          <span class="summary-ring__value">{{ nodePercent }}%</span>
        </div>
        <span class="summary-progress-label">{{ nodeProgressLabel }}</span>
      </template>
    </div>
  </section>
</template>

<style scoped>
.path-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3) var(--kapoj-space-5);
  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.summary-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.summary-author {
  color: var(--kapoj-text-secondary);
}

.summary-node-count {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.summary-problem-progress {
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.summary-completion {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  margin-left: auto;
}

.summary-ring {
  position: relative;
  display: grid;
  width: 46px;
  height: 46px;
  flex: 0 0 auto;
  place-items: center;
  background: conic-gradient(var(--kapoj-tree-branch-done) var(--progress), var(--kapoj-border) 0);
  border-radius: 50%;
  transition: background var(--kapoj-transition);
}

.summary-ring--empty {
  background: var(--kapoj-border);
}

.summary-ring::before {
  position: absolute;
  inset: 5px;
  content: '';
  background: var(--kapoj-bg-elevated);
  border-radius: 50%;
}

.summary-ring__value {
  position: relative;
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
}

.summary-progress-label {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.path-summary--compact {
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-2) var(--kapoj-space-3);
}

.path-summary--compact .summary-completion {
  width: 100%;
  margin-left: 0;
}

.path-summary--compact .summary-progress-bar {
  flex: 1 1 auto;
}

.path-summary--compact :deep(.p-progressbar) {
  height: 6px;
  background: var(--kapoj-border);
  border-radius: 999px;
}

.path-summary--compact :deep(.p-progressbar-value) {
  background: var(--kapoj-tree-branch-active);
}

@media (prefers-reduced-motion: reduce) {
  .summary-ring {
    transition: none;
  }
}
</style>
