<script setup lang="ts">
import { computed } from 'vue'

import type { LearningPathNodeItem } from '@/api/types'
import { learningStatusLabel, nodeProgressPercent } from '@/utils/learning'
import type { LearningTreeLayoutNode } from '@/utils/learningTreeLayout'

const props = defineProps<{
  layoutNode: LearningTreeLayoutNode
  selected: boolean
}>()

const emit = defineEmits<{
  select: [node: LearningPathNodeItem]
}>()

const progress = computed(() =>
  nodeProgressPercent(props.layoutNode.node.solvedCount, props.layoutNode.node.requiredCount),
)

const nodeStyle = computed(() => ({
  left: `${props.layoutNode.x}px`,
  top: `${props.layoutNode.y}px`,
  '--node-size': `${props.layoutNode.size}px`,
  '--progress': `${progress.value}%`,
}))

const ariaLabel = computed(() => {
  const node = props.layoutNode.node
  return `${node.title}, ${learningStatusLabel(node.status)}, ${node.solvedCount} / ${node.requiredCount}`
})

const titleInitial = computed(() => Array.from(props.layoutNode.node.title)[0] ?? '·')
</script>

<template>
  <button
    type="button"
    class="tree-node"
    :class="[`status-${layoutNode.node.status}`, { selected }]"
    :style="nodeStyle"
    :aria-label="ariaLabel"
    @click="emit('select', layoutNode.node)"
  >
    <span class="tree-node__surface" aria-hidden="true">
      <span class="tree-node__initial">{{ titleInitial }}</span>
      <span v-if="layoutNode.node.status === 'completed'" class="tree-node__status-mark">✓</span>
      <span v-else-if="layoutNode.node.status === 'locked'" class="tree-node__status-mark lock-mark"
        >⌑</span
      >
      <span
        v-if="layoutNode.children.length > 0 && layoutNode.node.problems.length > 0"
        class="tree-node__problem-count"
      >
        <svg viewBox="0 0 16 16" focusable="false">
          <path d="M13.6 2.5C7.1 2.4 3.6 5.1 3.6 9.2c0 2.1 1.5 3.3 3.4 3.3 4.3 0 6.1-4.9 6.6-10Z" />
          <path d="M2.7 14c2.2-3.3 4.7-5.5 8-7.2" />
        </svg>
        <span>{{ layoutNode.node.problems.length }}</span>
      </span>
    </span>
  </button>
</template>

<style scoped>
.tree-node {
  --node-accent: var(--kapoj-tree-branch-active);

  position: absolute;
  z-index: 2;
  display: grid;
  width: var(--node-size);
  height: var(--node-size);
  padding: 0;
  color: var(--kapoj-text);
  cursor: pointer;
  background: transparent;
  border: 0;
  border-radius: 50%;
  transform: translate(-50%, -50%);
  transition:
    filter var(--kapoj-transition),
    transform var(--kapoj-transition);
}

.tree-node:hover {
  z-index: 4;
  filter: brightness(1.12);
  transform: translate(-50%, -50%) scale(1.06);
}

.tree-node:focus-visible {
  outline: none;
  box-shadow: var(--kapoj-focus-ring);
}

.tree-node.selected {
  z-index: 5;
  filter: drop-shadow(0 0 12px var(--kapoj-tree-glow));
}

.tree-node.status-completed {
  --node-accent: var(--kapoj-tree-branch-done);
}

.tree-node.status-active {
  --node-accent: var(--kapoj-tree-branch-active);
}

.tree-node.status-locked {
  --node-accent: var(--kapoj-tree-branch-locked);
}

.tree-node__surface {
  position: relative;
  display: grid;
  width: 100%;
  height: 100%;
  place-items: center;
  background:
    radial-gradient(circle, var(--kapoj-surface-raised) 58%, transparent 60%),
    conic-gradient(var(--node-accent) var(--progress), var(--kapoj-border-strong) 0);
  border: 1px solid var(--node-accent);
  border-radius: 50%;
  box-shadow:
    inset 0 0 0 4px var(--kapoj-bg-elevated),
    0 5px 16px rgb(0 0 0 / 18%);
}

.status-locked .tree-node__surface {
  border-style: dashed;
  opacity: 0.82;
}

.tree-node__initial {
  max-width: 70%;
  overflow: hidden;
  color: var(--node-accent);
  font-size: calc(var(--node-size) * 0.36);
  font-weight: 700;
  line-height: 1;
  text-overflow: clip;
  white-space: nowrap;
}

.tree-node__status-mark {
  position: absolute;
  top: -2px;
  right: -2px;
  display: grid;
  width: 18px;
  height: 18px;
  color: var(--kapoj-bg);
  font-size: 12px;
  font-weight: 800;
  line-height: 1;
  place-items: center;
  background: var(--kapoj-tree-branch-done);
  border: 2px solid var(--kapoj-bg-elevated);
  border-radius: 50%;
}

.tree-node__status-mark.lock-mark {
  color: var(--kapoj-text-secondary);
  background: var(--kapoj-surface-raised);
  border-color: var(--kapoj-tree-branch-locked);
}

.tree-node__problem-count {
  position: absolute;
  right: -14px;
  bottom: -3px;
  display: flex;
  min-width: 25px;
  height: 21px;
  align-items: center;
  justify-content: center;
  gap: 2px;
  padding: 0 5px;
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  line-height: 1;
  background: var(--kapoj-surface-raised);
  border: 1px solid var(--kapoj-tree-branch);
  border-radius: 12px 12px 12px 3px;
  box-shadow: var(--kapoj-shadow-sm);
}

.tree-node__problem-count svg {
  width: 11px;
  height: 11px;
  fill: var(--kapoj-tree-foliage);
  stroke: var(--kapoj-tree-branch);
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 1.5;
}

@media (prefers-reduced-motion: reduce) {
  .tree-node {
    transition: none;
  }
}
</style>
