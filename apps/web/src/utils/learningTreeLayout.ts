import type { LearningNodeStatus, LearningPathNodeItem, LearningProblemSummary } from '@/api/types'

import { splitLearningProblems } from './learning'

export interface LearningTreeLayoutOptions {
  nodeSlotWidth?: number
  siblingGap?: number
  rootGap?: number
  levelGap?: number
  paddingX?: number
  paddingTop?: number
  paddingBottom?: number
  nodeSizeRoot?: number
  nodeSizeD1?: number
  nodeSizeD2?: number
  nodeSizeD3Plus?: number
}

export interface LearningTreeLayoutNode {
  node: LearningPathNodeItem
  id: number
  parentId: number | null
  depth: number
  x: number
  y: number
  size: number
  width: number
  children: LearningTreeLayoutNode[]
}

export interface LearningTreeLayoutEdge {
  parentId: number
  childId: number
  path: string
  status: LearningNodeStatus
}

export interface LearningTreeLayout {
  roots: LearningTreeLayoutNode[]
  nodes: LearningTreeLayoutNode[]
  edges: LearningTreeLayoutEdge[]
  width: number
  height: number
}

export interface LearningLeafPlacement {
  key: string
  problem: LearningProblemSummary | null
  moreCount: number
  x: number
  y: number
}

export const DEFAULT_LEARNING_TREE_LAYOUT: Required<LearningTreeLayoutOptions> = {
  nodeSlotWidth: 152,
  siblingGap: 28,
  rootGap: 56,
  levelGap: 126,
  paddingX: 96,
  paddingTop: 112,
  paddingBottom: 84,
  nodeSizeRoot: 60,
  nodeSizeD1: 54,
  nodeSizeD2: 46,
  nodeSizeD3Plus: 42,
}

const LEAF_WIDTH = 128
const LEAF_HEIGHT = 34
const LEAF_GAP = 8
const LEAF_RISE = 58
const LEAF_ARC_LIFT = 14
const SIDE_COLUMN_GAP = 16
const SIDE_ROW_GAP = 18
const LEAF_MARGIN = 16

export function buildLearningTreeLayout(
  nodes: LearningPathNodeItem[],
  options: LearningTreeLayoutOptions = {},
): LearningTreeLayout {
  const config: Required<LearningTreeLayoutOptions> = {
    ...DEFAULT_LEARNING_TREE_LAYOUT,
    ...options,
  }

  const roots = nodes.map((item) => measureSubtree(item, 0, null, config))
  const maxDepth = roots.reduce((max, root) => Math.max(max, subtreeMaxDepth(root)), 0)
  const height = config.paddingTop + maxDepth * config.levelGap + config.paddingBottom

  const rootsWidth =
    roots.reduce((sum, root) => sum + root.width, 0) +
    config.rootGap * Math.max(0, roots.length - 1)
  const width = rootsWidth + config.paddingX * 2

  let cursor = config.paddingX
  for (const root of roots) {
    placeSubtree(root, cursor, height, config)
    cursor += root.width + config.rootGap
  }

  const allNodes: LearningTreeLayoutNode[] = []
  const edges: LearningTreeLayoutEdge[] = []
  const collect = (layoutNode: LearningTreeLayoutNode) => {
    allNodes.push(layoutNode)
    for (const child of layoutNode.children) {
      edges.push({
        parentId: layoutNode.id,
        childId: child.id,
        path: buildLearningBranchPath(layoutNode.x, layoutNode.y, child.x, child.y),
        status: child.node.status,
      })
      collect(child)
    }
  }
  for (const root of roots) collect(root)

  return { roots, nodes: allNodes, edges, width, height }
}

export function buildLearningBranchPath(
  parentX: number,
  parentY: number,
  childX: number,
  childY: number,
): string {
  const dy = childY - parentY
  const dx = childX - parentX
  const bend = Math.min(64, Math.abs(dx) * 0.5)
  const direction = dx === 0 ? 0 : Math.sign(dx)
  const control1X = parentX + direction * bend
  const control2X = childX - direction * bend
  const control1Y = parentY + dy * 0.4
  const control2Y = parentY + dy * 0.6

  return `M${parentX} ${parentY}C${control1X} ${control1Y} ${control2X} ${control2Y} ${childX} ${childY}`
}

export function computeLearningTreeFitScale(
  canvasWidth: number,
  canvasHeight: number,
  viewportWidth: number,
  viewportHeight: number,
  minScale = 0.7,
): number {
  if (canvasWidth <= 0 || canvasHeight <= 0) return 1

  const availableWidth = Math.max(0, viewportWidth - 32)
  const availableHeight = Math.max(0, viewportHeight - 32)
  const scale = Math.min(1, availableWidth / canvasWidth, availableHeight / canvasHeight)

  return Math.max(minScale, scale)
}

export function buildLearningLeafPlacements(
  layoutNode: LearningTreeLayoutNode,
  problems: LearningProblemSummary[],
  canvasWidth: number,
  canvasHeight: number,
): LearningLeafPlacement[] {
  const { visible, more } = splitLearningProblems(problems)
  const entries: Array<Omit<LearningLeafPlacement, 'x' | 'y'>> = visible.map((problem) => ({
    key: `problem:${problem.problemId}`,
    problem,
    moreCount: 0,
  }))
  if (more > 0) {
    entries.push({ key: `more:${layoutNode.id}`, problem: null, moreCount: more })
  }
  if (entries.length === 0) return []

  const marginX = Math.min(LEAF_MARGIN, canvasWidth / 2)
  const marginY = Math.min(LEAF_MARGIN, canvasHeight / 2)
  const clampX = (value: number) => Math.max(marginX, Math.min(canvasWidth - marginX, value))
  const clampY = (value: number) => Math.max(marginY, Math.min(canvasHeight - marginY, value))

  const count = entries.length
  const totalWidth = count * LEAF_WIDTH + (count - 1) * LEAF_GAP
  const topY = layoutNode.y - layoutNode.size / 2 - LEAF_RISE - LEAF_ARC_LIFT - LEAF_HEIGHT / 2
  const fitsAbove = topY >= marginY && totalWidth <= canvasWidth - marginX * 2

  if (fitsAbove) {
    const left = Math.max(
      marginX,
      Math.min(canvasWidth - marginX - totalWidth, layoutNode.x - totalWidth / 2),
    )
    const center = (count - 1) / 2

    return entries.map((entry, index) => {
      const lift = LEAF_ARC_LIFT * (1 - Math.abs(index - center) / Math.max(center, 1))

      return {
        ...entry,
        x: clampX(left + LEAF_WIDTH / 2 + index * (LEAF_WIDTH + LEAF_GAP)),
        y: clampY(layoutNode.y - layoutNode.size / 2 - LEAF_RISE - lift),
      }
    })
  }

  const leftX = layoutNode.x - layoutNode.size / 2 - SIDE_COLUMN_GAP - LEAF_WIDTH / 2
  const rightX = layoutNode.x + layoutNode.size / 2 + SIDE_COLUMN_GAP + LEAF_WIDTH / 2

  return entries.map((entry, index) => {
    const row = Math.floor(index / 2)

    return {
      ...entry,
      x: clampX(index % 2 === 0 ? leftX : rightX),
      y: clampY(layoutNode.y + row * SIDE_ROW_GAP),
    }
  })
}

export function createSeededRandom(seed: number): () => number {
  let state = seed >>> 0

  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let value = state
    value = Math.imul(value ^ (value >>> 15), value | 1)
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61)
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296
  }
}

function nodeSizeForDepth(depth: number, config: Required<LearningTreeLayoutOptions>): number {
  if (depth === 0) return config.nodeSizeRoot
  if (depth === 1) return config.nodeSizeD1
  if (depth === 2) return config.nodeSizeD2
  return config.nodeSizeD3Plus
}

function measureSubtree(
  item: LearningPathNodeItem,
  depth: number,
  parentId: number | null,
  config: Required<LearningTreeLayoutOptions>,
): LearningTreeLayoutNode {
  const children = item.children.map((child) => measureSubtree(child, depth + 1, item.id, config))
  const childrenWidth =
    children.reduce((sum, child) => sum + child.width, 0) +
    config.siblingGap * Math.max(0, children.length - 1)
  const width =
    children.length === 0 ? config.nodeSlotWidth : Math.max(config.nodeSlotWidth, childrenWidth)

  return {
    node: item,
    id: item.id,
    parentId,
    depth,
    x: 0,
    y: 0,
    size: nodeSizeForDepth(depth, config),
    width,
    children,
  }
}

function subtreeMaxDepth(layoutNode: LearningTreeLayoutNode): number {
  return layoutNode.children.reduce(
    (max, child) => Math.max(max, subtreeMaxDepth(child)),
    layoutNode.depth,
  )
}

function placeSubtree(
  layoutNode: LearningTreeLayoutNode,
  left: number,
  height: number,
  config: Required<LearningTreeLayoutOptions>,
): void {
  layoutNode.y = height - config.paddingBottom - layoutNode.depth * config.levelGap

  if (layoutNode.children.length === 0) {
    layoutNode.x = left + layoutNode.width / 2
    return
  }

  let cursor = left
  for (const child of layoutNode.children) {
    placeSubtree(child, cursor, height, config)
    cursor += child.width + config.siblingGap
  }

  layoutNode.x =
    (layoutNode.children[0].x + layoutNode.children[layoutNode.children.length - 1].x) / 2
}
