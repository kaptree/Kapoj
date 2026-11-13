<script setup lang="ts">
import { computed } from 'vue'

import type { LearningPathNodeItem, LearningProblemSummary } from '@/api/types'
import type { LearningLeafPlacement, LearningTreeLayoutNode } from '@/utils/learningTreeLayout'
import { buildLearningLeafPlacements } from '@/utils/learningTreeLayout'

const props = defineProps<{
  layoutNode: LearningTreeLayoutNode
  problems: LearningProblemSummary[]
  canvasWidth: number
  canvasHeight: number
}>()

const emit = defineEmits<{
  more: [node: LearningPathNodeItem]
}>()

const placements = computed(() =>
  buildLearningLeafPlacements(
    props.layoutNode,
    props.problems,
    props.canvasWidth,
    props.canvasHeight,
  ),
)

const stemPath = (placement: LearningLeafPlacement) => {
  const startY = props.layoutNode.y - props.layoutNode.size / 2
  const endY = placement.y + 17
  const midY = (startY + endY) / 2
  return `M${props.layoutNode.x} ${startY} Q${props.layoutNode.x} ${midY} ${placement.x} ${endY}`
}

const placementStyle = (placement: LearningLeafPlacement) => ({
  left: `${placement.x}px`,
  top: `${placement.y}px`,
})
</script>

<template>
  <div class="tree-leaves" aria-label="题目列表">
    <svg
      class="tree-leaves__stems"
      :viewBox="`0 0 ${canvasWidth} ${canvasHeight}`"
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      <path v-for="placement in placements" :key="placement.key" :d="stemPath(placement)" />
    </svg>

    <template v-for="placement in placements" :key="placement.key">
      <RouterLink
        v-if="placement.problem"
        :to="`/problems/${placement.problem.problemId}`"
        class="tree-leaf"
        :class="{ solved: placement.problem.solved }"
        :style="placementStyle(placement)"
        :title="placement.problem.title"
        @click.stop
      >
        <span class="tree-leaf__state" aria-hidden="true">{{
          placement.problem.solved ? '✓' : '·'
        }}</span>
        <span class="tree-leaf__title">{{ placement.problem.title }}</span>
      </RouterLink>
      <button
        v-else
        type="button"
        class="tree-leaf tree-leaf--more"
        :style="placementStyle(placement)"
        :aria-label="`查看其余 ${placement.moreCount} 道题目`"
        @click.stop="emit('more', layoutNode.node)"
      >
        +{{ placement.moreCount }}
      </button>
    </template>
  </div>
</template>

<style scoped>
.tree-leaves {
  position: absolute;
  z-index: 3;
  inset: 0;
  pointer-events: none;
}

.tree-leaves__stems {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: visible;
  pointer-events: none;
}

.tree-leaves__stems path {
  fill: none;
  stroke: var(--kapoj-tree-branch);
  stroke-dasharray: 3 5;
  stroke-linecap: round;
  stroke-width: 1.5;
  opacity: 0.75;
}

.tree-leaf {
  position: absolute;
  display: flex;
  width: 128px;
  height: 34px;
  align-items: center;
  gap: 7px;
  overflow: hidden;
  padding: 0 10px;
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  line-height: 1;
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
  pointer-events: auto;
  background: color-mix(in srgb, var(--kapoj-surface-raised) 94%, var(--kapoj-tree-branch));
  border: 1px solid var(--kapoj-tree-branch-locked);
  border-radius: 18px 18px 18px 5px;
  box-shadow: var(--kapoj-shadow-sm);
  transform: translate(-50%, -50%);
  transition:
    color var(--kapoj-transition),
    border-color var(--kapoj-transition),
    background-color var(--kapoj-transition);
}

.tree-leaf:hover,
.tree-leaf:focus-visible {
  color: var(--kapoj-text);
  border-color: var(--kapoj-primary);
  outline: none;
  box-shadow: var(--kapoj-focus-ring);
}

.tree-leaf.solved {
  color: var(--kapoj-tree-branch-done);
  background: color-mix(in srgb, var(--kapoj-surface-raised) 88%, var(--kapoj-tree-branch-done));
  border-color: var(--kapoj-tree-branch-done);
}

.tree-leaf__state {
  flex: 0 0 auto;
  color: var(--kapoj-tree-branch-active);
  font-size: 14px;
  font-weight: 800;
}

.tree-leaf.solved .tree-leaf__state {
  color: var(--kapoj-tree-branch-done);
}

.tree-leaf__title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tree-leaf--more {
  justify-content: center;
  padding: 0 8px;
  color: var(--kapoj-tree-branch-active);
  font-family: var(--kapoj-font-mono);
  background: var(--kapoj-surface-raised);
  border-color: var(--kapoj-tree-branch-active);
}

@media (prefers-reduced-motion: reduce) {
  .tree-leaf {
    transition: none;
  }
}
</style>
