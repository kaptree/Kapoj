<script setup lang="ts">
import { computed } from 'vue'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'

import type { LearningPathNodeItem } from '@/api/types'
import LearningNodePanel from '@/components/learning/LearningNodePanel.vue'
import {
  flattenLearningNodesWithDepth,
  learningStatusLabel,
  learningStatusSeverity,
  nodeProgressPercent,
  splitLearningProblems,
} from '@/utils/learning'

const props = defineProps<{
  nodes: LearningPathNodeItem[]
  selectedId: number | null
}>()

const emit = defineEmits<{
  select: [node: LearningPathNodeItem]
}>()

const rows = computed(() =>
  flattenLearningNodesWithDepth(props.nodes).map(({ node, depth }, index) => {
    const selected = node.id === props.selectedId
    const isLeaf = node.children.length === 0
    return {
      node,
      depth,
      index: index + 1,
      selected,
      progress: nodeProgressPercent(node.solvedCount, node.requiredCount),
      problemSplit:
        selected && isLeaf && node.problems.length > 0
          ? splitLearningProblems(node.problems, 5)
          : null,
    }
  }),
)
</script>

<template>
  <section class="mobile-tree" aria-label="学习路线树">
    <ol class="mobile-tree__list" role="tree">
      <li
        v-for="row in rows"
        :key="row.node.id"
        class="mobile-row"
        role="treeitem"
        :aria-level="row.depth + 1"
        :aria-selected="row.selected"
        :data-depth="row.depth"
        :style="{ '--depth': row.depth, '--indent': `${row.depth * 22}px` }"
      >
        <button
          type="button"
          class="mobile-node"
          :class="[`status-${row.node.status}`, { selected: row.selected }]"
          :data-node-id="row.node.id"
          :aria-label="`${row.node.title}, ${learningStatusLabel(row.node.status)}, ${row.node.solvedCount} / ${row.node.requiredCount}`"
          @click="emit('select', row.node)"
        >
          <span class="mobile-node__title-line">
            <span class="mobile-node__status-dot" aria-hidden="true" />
            <span class="mobile-node__title">{{ row.node.title }}</span>
            <span v-if="row.node.children.length" class="mobile-node__leaf-count">
              {{ row.node.problems.length }} 题
            </span>
          </span>
          <span class="mobile-node__status-line">
            <Tag
              :value="learningStatusLabel(row.node.status)"
              :severity="learningStatusSeverity(row.node.status)"
              class="mobile-node__status"
            />
            <span class="mobile-node__progress-count">
              {{ row.node.solvedCount }} / {{ row.node.requiredCount }}
            </span>
          </span>
          <ProgressBar
            :value="row.progress"
            :show-value="false"
            class="mobile-node__progress"
            :aria-label="`节点进度 ${row.node.solvedCount} / ${row.node.requiredCount}`"
          />
        </button>

        <div v-if="row.problemSplit" class="mobile-leaves" aria-label="题目列表">
          <RouterLink
            v-for="problem in row.problemSplit.visible"
            :key="problem.problemId"
            :to="`/problems/${problem.problemId}`"
            class="mobile-leaf"
            :class="{ solved: problem.solved }"
            :title="problem.title"
          >
            {{ problem.title }}
          </RouterLink>
          <span v-if="row.problemSplit.more" class="mobile-leaf-more">
            +{{ row.problemSplit.more }}
          </span>
        </div>

        <LearningNodePanel
          v-if="row.selected"
          class="mobile-node-panel"
          :node="row.node"
          :index="row.index"
        />
      </li>
    </ol>
  </section>
</template>

<style scoped>
.mobile-tree {
  width: 100%;
  min-width: 0;
}

.mobile-tree__list {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}

.mobile-row {
  position: relative;
  min-width: 0;
}

.mobile-row:not([data-depth='0'])::before {
  position: absolute;
  top: 0;
  bottom: calc(100% - 31px);
  left: calc(var(--indent) - 11px);
  width: 2px;
  content: '';
  background: var(--kapoj-tree-branch);
}

.mobile-row:not([data-depth='0'])::after {
  position: absolute;
  top: 29px;
  left: calc(var(--indent) - 11px);
  width: 22px;
  height: 2px;
  content: '';
  background: var(--kapoj-tree-branch);
}

.mobile-node {
  --node-accent: var(--kapoj-tree-branch-active);

  position: relative;
  display: flex;
  width: calc(100% - var(--indent));
  min-width: 0;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  margin-inline-start: var(--indent);
  padding: var(--kapoj-space-3);
  color: var(--kapoj-text);
  text-align: start;
  cursor: pointer;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-inline-start: 3px solid var(--node-accent);
  border-radius: var(--kapoj-radius-md);
  transition:
    border-color var(--kapoj-transition),
    box-shadow var(--kapoj-transition);
}

.mobile-node.status-completed {
  --node-accent: var(--kapoj-tree-branch-done);
}

.mobile-node.status-locked {
  --node-accent: var(--kapoj-tree-branch-locked);
}

.mobile-node.selected {
  border-color: var(--node-accent);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--node-accent) 18%, transparent);
}

.mobile-node:hover,
.mobile-node:focus-visible {
  border-color: var(--node-accent);
  outline: none;
  box-shadow: var(--kapoj-focus-ring);
}

.mobile-node__title-line,
.mobile-node__status-line {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.mobile-node__status-dot {
  width: 9px;
  height: 9px;
  flex: 0 0 auto;
  background: var(--node-accent);
  border-radius: 50%;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--node-accent) 18%, transparent);
}

.mobile-node__title {
  min-width: 0;
  flex: 1 1 auto;
  overflow-wrap: anywhere;
  font-weight: 600;
}

.mobile-node__leaf-count {
  flex: 0 0 auto;
  padding: 2px 7px;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  white-space: nowrap;
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: 999px;
}

.mobile-node__status-line {
  justify-content: space-between;
}

.mobile-node__status {
  font-size: 11px;
}

.mobile-node__progress-count {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.mobile-node__progress :deep(.p-progressbar) {
  height: 5px;
  background: var(--kapoj-border);
  border-radius: 999px;
}

.mobile-node__progress :deep(.p-progressbar-value) {
  background: var(--node-accent);
}

.mobile-leaves,
.mobile-node-panel {
  width: calc(100% - var(--indent));
  margin-inline-start: var(--indent);
}

.mobile-leaves {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-2) 0;
}

.mobile-leaf,
.mobile-leaf-more {
  display: inline-flex;
  max-width: 100%;
  min-height: 30px;
  align-items: center;
  padding: 0 var(--kapoj-space-3);
  overflow: hidden;
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: 999px;
}

.mobile-leaf:hover,
.mobile-leaf:focus-visible {
  color: var(--kapoj-text);
  border-color: var(--kapoj-primary);
  outline: none;
  box-shadow: var(--kapoj-focus-ring);
}

.mobile-leaf.solved {
  color: var(--kapoj-tree-branch-done);
  border-color: var(--kapoj-tree-branch-done);
}

.mobile-leaf-more {
  color: var(--kapoj-tree-branch-active);
  font-family: var(--kapoj-font-mono);
}

.mobile-node-panel :deep(.node-panel) {
  height: auto;
  overflow: visible;
}

@media (prefers-reduced-motion: reduce) {
  .mobile-node {
    transition: none;
  }
}
</style>
