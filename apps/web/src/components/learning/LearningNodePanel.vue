<script setup lang="ts">
import { computed } from 'vue'
import Button from 'primevue/button'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'

import type { LearningPathNodeItem } from '@/api/types'
import MarkdownView from '@/components/MarkdownView.vue'
import { useTheme } from '@/theme/theme'
import { learningStatusLabel, learningStatusSeverity, nodeProgressPercent } from '@/utils/learning'

const props = withDefaults(
  defineProps<{
    node: LearningPathNodeItem | null
    index?: number | null
    dismissible?: boolean
    emptyLabel?: string
  }>(),
  {
    index: null,
    dismissible: false,
    emptyLabel: '选择树上的节点查看详情。',
  },
)

const emit = defineEmits<{
  close: []
}>()

const { isDark } = useTheme()

const progress = computed(() => {
  if (!props.node) return 0
  return nodeProgressPercent(props.node.solvedCount, props.node.requiredCount)
})

const eyebrow = computed(() => {
  if (props.index == null) return 'NODE'
  return `NODE // ${String(props.index).padStart(2, '0')}`
})

const nextProblem = computed(() => {
  const problems = props.node?.problems ?? []
  return problems.find((problem) => !problem.solved) ?? problems[0] ?? null
})

const allSolved = computed(() => {
  const problems = props.node?.problems ?? []
  return problems.length > 0 && problems.every((problem) => problem.solved)
})

const actionLabel = computed(() => {
  if (!props.node?.problems.length) return '暂无题目'
  return allSolved.value ? '复习题目' : '继续挑战'
})

const actionTarget = computed(() =>
  nextProblem.value ? `/problems/${nextProblem.value.problemId}` : null,
)

const progressHint = computed(() => {
  if (!props.node) return ''
  if (props.node.status === 'completed') return `完成时 ${props.node.solvedCount} 题`
  const remaining = Math.max(0, props.node.requiredCount - props.node.solvedCount)
  return `已通过 ${props.node.solvedCount} 题 / 还需 ${remaining} 题`
})
</script>

<template>
  <aside class="node-panel" aria-label="节点卷宗">
    <header class="panel-head">
      <span class="panel-eyebrow">{{ eyebrow }}</span>
      <Button
        v-if="dismissible"
        class="panel-close"
        icon="pi pi-times"
        text
        rounded
        size="small"
        aria-label="关闭节点卷宗"
        @click="emit('close')"
      />
    </header>

    <p v-if="!node" class="panel-empty">{{ emptyLabel }}</p>

    <template v-else>
      <div class="panel-status-row">
        <Tag
          :value="learningStatusLabel(node.status)"
          :severity="learningStatusSeverity(node.status)"
          :icon="node.status === 'locked' ? 'pi pi-lock' : undefined"
          class="panel-status"
        />
        <span class="panel-progress-count">{{ node.solvedCount }} / {{ node.requiredCount }}</span>
      </div>

      <h2 class="panel-title">{{ node.title }}</h2>

      <MarkdownView
        v-if="node.descriptionMd"
        :source="node.descriptionMd"
        :theme="isDark ? 'dark' : 'light'"
        class="panel-description"
      />
      <p v-else class="panel-description-empty">该节点暂无描述</p>

      <div class="panel-progress">
        <ProgressBar
          :value="progress"
          :show-value="false"
          class="panel-progress-bar"
          :aria-label="`节点进度 ${node.solvedCount} / ${node.requiredCount}`"
        />
        <span class="panel-progress-hint">{{ progressHint }}</span>
      </div>

      <p v-if="node.status === 'locked'" class="panel-locked-hint">
        完成前置节点后推进进度；题目仍可提前挑战。
      </p>

      <ul v-if="node.problems.length" class="panel-problems">
        <li v-for="problem in node.problems" :key="problem.problemId">
          <RouterLink
            :to="`/problems/${problem.problemId}`"
            class="panel-problem"
            :class="{ solved: problem.solved }"
          >
            <span class="panel-problem__state" aria-hidden="true">{{
              problem.solved ? '✓' : '○'
            }}</span>
            <span class="panel-problem__title">{{ problem.title }}</span>
            <span class="panel-problem__difficulty">难度 {{ problem.difficulty }}</span>
            <span class="panel-problem__status">{{ problem.solved ? '已通过' : '未开始' }}</span>
          </RouterLink>
        </li>
      </ul>
      <p v-else class="panel-problems-empty">该节点暂无题目</p>

      <footer class="panel-foot">
        <Button
          v-if="actionTarget"
          as="router-link"
          :to="actionTarget"
          :label="actionLabel"
          icon="pi pi-arrow-right"
          icon-pos="right"
          class="panel-action"
        />
        <Button v-else class="panel-action" :label="actionLabel" disabled />
      </footer>
    </template>
  </aside>
</template>

<style scoped>
.node-panel {
  display: flex;
  min-width: 0;
  height: 100%;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-4);
  overflow-y: auto;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.panel-eyebrow {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.08em;
}

.panel-status-row {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.panel-progress-count {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.panel-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.1rem;
  font-weight: 600;
  line-height: 1.35;
  overflow-wrap: anywhere;
}

.panel-description {
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.panel-description-empty,
.panel-problems-empty,
.panel-empty {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 0.875rem;
}

.panel-empty {
  margin-top: var(--kapoj-space-2);
}

.panel-progress {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
}

.panel-progress :deep(.p-progressbar) {
  height: 6px;
  background: var(--kapoj-border);
  border-radius: 999px;
}

.panel-progress :deep(.p-progressbar-value) {
  background: var(--kapoj-tree-branch-active);
}

.panel-progress-hint {
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.panel-locked-hint {
  margin: 0;
  padding: var(--kapoj-space-2) var(--kapoj-space-3);
  color: var(--kapoj-text-muted);
  font-size: 12px;
  line-height: 1.5;
  background: var(--kapoj-surface);
  border-left: 2px solid var(--kapoj-tree-branch-locked);
  border-radius: var(--kapoj-radius-sm);
}

.panel-problems {
  display: flex;
  max-height: 22rem;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
}

.panel-problem {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-2) var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  text-decoration: none;
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
  transition:
    color var(--kapoj-transition),
    border-color var(--kapoj-transition),
    background-color var(--kapoj-transition);
}

.panel-problem:hover,
.panel-problem:focus-visible {
  color: var(--kapoj-text);
  border-color: var(--kapoj-primary);
  outline: none;
  box-shadow: var(--kapoj-focus-ring);
}

.panel-problem.solved {
  color: var(--kapoj-tree-branch-done);
  border-color: color-mix(in srgb, var(--kapoj-tree-branch-done) 45%, var(--kapoj-border));
}

.panel-problem__state {
  color: var(--kapoj-tree-branch-active);
  font-size: 13px;
  font-weight: 800;
  line-height: 1;
}

.panel-problem.solved .panel-problem__state {
  color: var(--kapoj-tree-branch-done);
}

.panel-problem__title {
  min-width: 0;
  color: inherit;
  overflow-wrap: anywhere;
  white-space: normal;
}

.panel-problem__difficulty {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.panel-problem__status {
  grid-column: 2 / -1;
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.panel-problem.solved .panel-problem__status {
  color: var(--kapoj-tree-branch-done);
}

.panel-foot {
  margin-top: auto;
  padding-top: var(--kapoj-space-2);
}

.panel-action {
  width: 100%;
}

@media (prefers-reduced-motion: reduce) {
  .panel-problem {
    transition: none;
  }
}
</style>
