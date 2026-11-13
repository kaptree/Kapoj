<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'

import type { LearningPathNodeItem, LearningNodeStatus } from '@/api/types'
import LearningTreeLeaves from './LearningTreeLeaves.vue'
import LearningTreeNode from './LearningTreeNode.vue'
import {
  buildLearningTreeLayout,
  computeLearningTreeFitScale,
  createSeededRandom,
} from '@/utils/learningTreeLayout'

const props = defineProps<{
  nodes: LearningPathNodeItem[]
  pathId: number
  selectedId: number | null
}>()

const emit = defineEmits<{
  select: [node: LearningPathNodeItem]
  more: [node: LearningPathNodeItem]
}>()

const viewport = ref<HTMLDivElement | null>(null)
const viewportSize = ref({ width: 0, height: 0 })
const gridId = `tree-grid-${useId()}`
const layout = computed(() => buildLearningTreeLayout(props.nodes))
const scale = computed(() =>
  computeLearningTreeFitScale(
    layout.value.width,
    layout.value.height,
    viewportSize.value.width,
    viewportSize.value.height,
  ),
)

interface FoliageDot {
  x: number
  y: number
  radius: number
  sparkle: boolean
}

interface TreeCrown {
  x: number
  y: number
  rx: number
  ry: number
  dots: FoliageDot[]
}

function topY(layoutNode: (typeof layout.value.roots)[number]): number {
  return layoutNode.children.reduce((top, child) => Math.min(top, topY(child)), layoutNode.y)
}

const crowns = computed<TreeCrown[]>(() => {
  const random = createSeededRandom(props.pathId)

  return layout.value.roots.map((root) => {
    const rx = Math.max(46, Math.min(170, root.width * 0.44))
    const ry = 42
    const x = root.x
    const y = Math.max(ry + 12, topY(root) - 46)
    const dots = Array.from({ length: 28 }, (_, index) => {
      const angle = random() * Math.PI * 2
      const radius = Math.sqrt(random())
      return {
        x: x + Math.cos(angle) * rx * radius * 0.86,
        y: y + Math.sin(angle) * ry * radius * 0.72,
        radius: 1.2 + random() * 2.4,
        sparkle: index % 7 === 0,
      }
    })

    return { x, y, rx, ry, dots }
  })
})

const selectedLeaf = computed(() =>
  layout.value.nodes.find((item) => item.id === props.selectedId && item.children.length === 0),
)

const legendItems: Array<{ status: LearningNodeStatus; label: string }> = [
  { status: 'completed', label: '已完成' },
  { status: 'active', label: '进行中' },
  { status: 'locked', label: '未解锁' },
]

let resizeObserver: ResizeObserver | undefined

function updateViewportSize() {
  if (!viewport.value) return
  viewportSize.value = {
    width: viewport.value.clientWidth,
    height: viewport.value.clientHeight,
  }
}

function centerSelectedNode() {
  const container = viewport.value
  if (!container || props.selectedId === null) return
  const selectedNode = layout.value.nodes.find((item) => item.id === props.selectedId)
  if (!selectedNode) return

  container.scrollLeft = Math.max(0, selectedNode.x * scale.value - container.clientWidth / 2)
  container.scrollTop = Math.max(0, selectedNode.y * scale.value - container.clientHeight / 2)
}

watch(
  [() => props.selectedId, scale],
  () => {
    void nextTick(centerSelectedNode)
  },
  { flush: 'post', immediate: true },
)

onMounted(async () => {
  await nextTick()
  updateViewportSize()
  if (typeof ResizeObserver !== 'undefined' && viewport.value) {
    resizeObserver = new ResizeObserver(updateViewportSize)
    resizeObserver.observe(viewport.value)
  }
  centerSelectedNode()
})

onBeforeUnmount(() => resizeObserver?.disconnect())
</script>

<template>
  <section class="learning-tree-canvas">
    <div ref="viewport" class="tree-viewport" role="group" aria-label="学习路线树">
      <div
        class="tree-scaler"
        :style="{
          width: `${layout.width * scale}px`,
          height: `${layout.height * scale}px`,
        }"
      >
        <div
          class="tree-stage"
          :style="{
            width: `${layout.width}px`,
            height: `${layout.height}px`,
            transform: `scale(${scale})`,
          }"
        >
          <svg
            class="tree-decoration"
            :viewBox="`0 0 ${layout.width} ${layout.height}`"
            preserveAspectRatio="none"
            aria-hidden="true"
          >
            <defs>
              <pattern :id="gridId" width="28" height="28" patternUnits="userSpaceOnUse">
                <path d="M 28 0 L 0 0 0 28" class="tree-decoration__grid-line" />
              </pattern>
            </defs>
            <rect width="100%" height="100%" :fill="`url(#${gridId})`" />
            <g class="tree-trunks">
              <path
                v-for="root in layout.roots"
                :key="`trunk-${root.id}`"
                :class="`status-${root.node.status}`"
                :d="`M${root.x} ${layout.height + 8} C${root.x - 5} ${layout.height - 20} ${root.x + 4} ${root.y + root.size} ${root.x} ${root.y + root.size / 2}`"
              />
            </g>
            <g class="tree-crowns">
              <g v-for="(crown, index) in crowns" :key="`crown-${index}`">
                <ellipse :cx="crown.x" :cy="crown.y" :rx="crown.rx" :ry="crown.ry" />
                <circle
                  v-for="(dot, dotIndex) in crown.dots"
                  :key="dotIndex"
                  :cx="dot.x"
                  :cy="dot.y"
                  :r="dot.radius"
                  :class="{ sparkle: dot.sparkle }"
                />
              </g>
            </g>
          </svg>

          <svg
            class="tree-connectors"
            :viewBox="`0 0 ${layout.width} ${layout.height}`"
            preserveAspectRatio="none"
            aria-hidden="true"
          >
            <path
              v-for="edge in layout.edges"
              :key="`${edge.parentId}-${edge.childId}`"
              :d="edge.path"
              :class="`tree-edge status-${edge.status}`"
            />
          </svg>

          <LearningTreeNode
            v-for="layoutNode in layout.nodes"
            :key="layoutNode.id"
            :layout-node="layoutNode"
            :selected="layoutNode.id === selectedId"
            @select="emit('select', $event)"
          />

          <LearningTreeLeaves
            v-if="selectedLeaf"
            :layout-node="selectedLeaf"
            :problems="selectedLeaf.node.problems"
            :canvas-width="layout.width"
            :canvas-height="layout.height"
            @more="emit('more', $event)"
          />
        </div>
      </div>
    </div>

    <ul class="tree-legend" aria-label="节点状态图例">
      <li v-for="item in legendItems" :key="item.status" :class="`status-${item.status}`">
        <span class="tree-legend__dot" aria-hidden="true" />
        {{ item.label }}
      </li>
      <li class="tree-legend__leaf-item">
        <span class="tree-legend__leaf" aria-hidden="true">✓</span>
        已通过题目
      </li>
    </ul>
  </section>
</template>

<style scoped>
.learning-tree-canvas {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.tree-viewport {
  position: relative;
  width: 100%;
  height: min(74vh, 760px);
  min-height: 380px;
  overflow: auto;
  overscroll-behavior: contain;
  background:
    radial-gradient(ellipse at 50% 8%, var(--kapoj-tree-glow), transparent 46%),
    var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-lg);
  scrollbar-color: var(--kapoj-border-strong) transparent;
  scrollbar-width: thin;
}

.tree-scaler {
  position: relative;
  flex: none;
  margin-inline: auto;
}

.tree-stage {
  position: absolute;
  inset: 0 auto auto 0;
  overflow: hidden;
  transform-origin: top left;
}

.tree-decoration,
.tree-connectors {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: visible;
  pointer-events: none;
}

.tree-decoration {
  z-index: 0;
}

.tree-decoration__grid-line {
  fill: none;
  stroke: var(--kapoj-tree-grid);
  stroke-width: 0.8;
}

.tree-trunks path {
  fill: none;
  stroke: var(--kapoj-tree-trunk);
  stroke-linecap: round;
  stroke-width: 8;
  opacity: 0.72;
}

.tree-trunks .status-completed {
  stroke: var(--kapoj-tree-branch-done);
}

.tree-trunks .status-active {
  stroke: var(--kapoj-tree-branch-active);
}

.tree-trunks .status-locked {
  stroke: var(--kapoj-tree-branch-locked);
  stroke-dasharray: 4 6;
}

.tree-crowns ellipse {
  fill: var(--kapoj-tree-foliage);
  stroke: var(--kapoj-tree-branch);
  stroke-width: 0.8;
  opacity: 0.5;
}

.tree-crowns circle {
  fill: var(--kapoj-tree-foliage);
  opacity: 0.68;
}

.tree-crowns circle.sparkle {
  fill: var(--kapoj-tree-glow);
  filter: drop-shadow(0 0 5px var(--kapoj-tree-glow));
  animation: tree-sparkle 3.4s ease-in-out infinite alternate;
}

.tree-connectors {
  z-index: 1;
}

.tree-edge {
  fill: none;
  stroke: var(--kapoj-tree-branch);
  stroke-linecap: round;
  stroke-width: 4;
  opacity: 0.68;
}

.tree-edge.status-completed {
  stroke: var(--kapoj-tree-branch-done);
}

.tree-edge.status-active {
  stroke: var(--kapoj-tree-branch-active);
  stroke-dasharray: 7 5;
  animation: tree-flow 3s linear infinite;
}

.tree-edge.status-locked {
  stroke: var(--kapoj-tree-branch-locked);
  stroke-dasharray: 3 7;
}

.tree-legend {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2) var(--kapoj-space-4);
  margin: 0;
  padding: 0 var(--kapoj-space-1);
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  list-style: none;
}

.tree-legend li {
  display: inline-flex;
  align-items: center;
  gap: 7px;
}

.tree-legend__dot {
  width: 9px;
  height: 9px;
  background: var(--kapoj-tree-branch-active);
  border-radius: 50%;
  box-shadow: 0 0 8px var(--kapoj-tree-glow);
}

.tree-legend .status-completed .tree-legend__dot {
  background: var(--kapoj-tree-branch-done);
}

.tree-legend .status-locked .tree-legend__dot {
  background: var(--kapoj-tree-branch-locked);
}

.tree-legend__leaf {
  display: inline-grid;
  width: 16px;
  height: 16px;
  color: var(--kapoj-tree-branch-done);
  font-size: 10px;
  font-weight: 800;
  place-items: center;
  border: 1px solid var(--kapoj-tree-branch-done);
  border-radius: 50%;
}

@keyframes tree-flow {
  to {
    stroke-dashoffset: -24;
  }
}

@keyframes tree-sparkle {
  from {
    opacity: 0.32;
  }

  to {
    opacity: 0.9;
  }
}

@media (max-width: 640px) {
  .tree-viewport {
    height: min(68vh, 620px);
    min-height: 360px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .tree-edge.status-active,
  .tree-crowns circle.sparkle {
    animation: none;
  }
}
</style>
