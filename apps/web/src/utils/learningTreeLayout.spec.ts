import { describe, expect, it } from 'vitest'

import type { LearningPathNodeItem, LearningProblemSummary } from '@/api/types'
import {
  DEFAULT_LEARNING_TREE_LAYOUT,
  buildLearningBranchPath,
  buildLearningLeafPlacements,
  buildLearningTreeLayout,
  computeLearningTreeFitScale,
  createSeededRandom,
  type LearningLeafPlacement,
  type LearningTreeLayoutNode,
} from './learningTreeLayout'

// Keep in sync with the leaf box dimensions in learningTreeLayout.ts.
const LEAF_WIDTH = 128
const LEAF_HEIGHT = 34

describe('learning tree layout', () => {
  it('places a parent at the midpoint of its children and colors edges by child status', () => {
    const layout = buildLearningTreeLayout([
      node({
        id: 1,
        title: '根',
        status: 'active',
        children: [
          node({ id: 2, parentId: 1, title: '左', status: 'completed' }),
          node({ id: 3, parentId: 1, title: '右', status: 'locked' }),
        ],
      }),
    ])
    const root = layout.nodes.find((n) => n.id === 1)!
    const left = layout.nodes.find((n) => n.id === 2)!
    const right = layout.nodes.find((n) => n.id === 3)!

    expect(root.x).toBe((left.x + right.x) / 2)
    expect(left.x).toBeLessThan(right.x)
    expect(root.y).toBeGreaterThan(left.y)
    expect(layout.edges.find((e) => e.childId === 2)!.status).toBe('completed')
    expect(layout.edges.find((e) => e.childId === 3)!.status).toBe('locked')
  })

  it('builds multiple roots without overlap', () => {
    const layout = buildLearningTreeLayout([
      node({ id: 1, children: [node({ id: 2, parentId: 1 })] }),
      node({ id: 3, children: [node({ id: 4, parentId: 3 })] }),
    ])

    expect(layout.roots).toHaveLength(2)

    const first = layout.nodes.filter((n) => n.id === 1 || n.id === 2)
    const second = layout.nodes.filter((n) => n.id === 3 || n.id === 4)

    expect(Math.max(...first.map((n) => n.x))).toBeLessThan(Math.min(...second.map((n) => n.x)))
  })

  it('handles depth 8 and assigns smaller node sizes', () => {
    const chain = Array.from({ length: 8 }, (_, index) => index).reduceRight<
      LearningPathNodeItem[]
    >((children, id) => [node({ id, children })], [])

    const layout = buildLearningTreeLayout(chain)
    const deepest = layout.nodes.find((n) => n.id === 7)!

    expect(layout.height).toBe(
      DEFAULT_LEARNING_TREE_LAYOUT.paddingTop +
        7 * DEFAULT_LEARNING_TREE_LAYOUT.levelGap +
        DEFAULT_LEARNING_TREE_LAYOUT.paddingBottom,
    )
    expect(deepest.depth).toBe(7)
    expect(deepest.size).toBe(DEFAULT_LEARNING_TREE_LAYOUT.nodeSizeD3Plus)
  })

  it('builds a cubic branch path', () => {
    const path = buildLearningBranchPath(10, 100, 30, 20)

    expect(path.startsWith('M10 100C')).toBe(true)
    expect(path).toContain('30 20')
  })

  it('fit scale clamps to 1 and 0.7', () => {
    expect(computeLearningTreeFitScale(100, 100, 500, 500)).toBe(1)
    expect(computeLearningTreeFitScale(5000, 5000, 800, 600)).toBe(0.7)
    expect(computeLearningTreeFitScale(1000, 500, 800, 600)).toBeCloseTo(0.768, 5)
  })

  it('leaf placements fall back to the sides near the top edge', () => {
    const nearTop = layoutNode({ y: 20 })
    const placements = buildLearningLeafPlacements(nearTop, problems(3), 640, 480)

    for (const placement of placements) expectLeafInside(placement, 640, 480)

    expect(placements.some((placement) => placement.y >= nearTop.y)).toBe(true)
  })

  it('spaces side-column rows by at least a leaf height', () => {
    const placements = buildLearningLeafPlacements(layoutNode({ y: 20 }), problems(7), 640, 480)

    for (const placement of placements) expectLeafInside(placement, 640, 480)

    const columns = [
      placements.filter((_, index) => index % 2 === 0),
      placements.filter((_, index) => index % 2 === 1),
    ]
    for (const column of columns) {
      for (let index = 1; index < column.length; index += 1) {
        expect(column[index].y - column[index - 1].y).toBeGreaterThanOrEqual(LEAF_HEIGHT)
      }
    }
  })

  it('keeps full leaf boxes inside the canvas when clamped', () => {
    const placements = buildLearningLeafPlacements(
      layoutNode({ x: 8, y: 20 }),
      problems(4),
      300,
      240,
    )

    for (const placement of placements) expectLeafInside(placement, 300, 240)
  })

  it('centers leaves on canvases smaller than a leaf', () => {
    const placements = buildLearningLeafPlacements(
      layoutNode({ x: 10, y: 10 }),
      problems(2),
      100,
      60,
    )

    expect(placements).toHaveLength(2)
    expect(placements.every((placement) => placement.x === 50)).toBe(true)
  })

  it('leaf placements cap at 4 + more', () => {
    const placements = buildLearningLeafPlacements(layoutNode({ y: 400 }), problems(7), 1200, 800)

    expect(placements).toHaveLength(5)
    expect(placements.at(-1)).toMatchObject({ problem: null, moreCount: 3 })
    for (const placement of placements) expectLeafInside(placement, 1200, 800)
  })

  it('shows every problem when there are five or fewer', () => {
    const placements = buildLearningLeafPlacements(layoutNode({ y: 400 }), problems(5), 1200, 800)

    expect(placements).toHaveLength(5)
    expect(placements.every((placement) => placement.moreCount === 0)).toBe(true)
  })

  it('seeded random is stable', () => {
    const first = createSeededRandom(42)
    const second = createSeededRandom(42)

    expect([first(), first(), first()]).toEqual([second(), second(), second()])
    expect(createSeededRandom(1)()).not.toBe(createSeededRandom(2)())
  })
})

function node(overrides: Partial<LearningPathNodeItem> = {}): LearningPathNodeItem {
  return {
    id: 1,
    parentId: null,
    title: '节点',
    descriptionMd: '',
    problemIds: [],
    problems: [],
    sortOrder: 0,
    requiredCount: 1,
    solvedCount: 0,
    status: 'active',
    children: [],
    ...overrides,
  }
}

function layoutNode(overrides: Partial<LearningTreeLayoutNode> = {}): LearningTreeLayoutNode {
  const item = node({ id: 1 })

  return {
    node: item,
    id: item.id,
    parentId: null,
    depth: 0,
    x: 320,
    y: 400,
    size: DEFAULT_LEARNING_TREE_LAYOUT.nodeSizeRoot,
    width: DEFAULT_LEARNING_TREE_LAYOUT.nodeSlotWidth,
    children: [],
    ...overrides,
  }
}

function expectLeafInside(
  placement: LearningLeafPlacement,
  canvasWidth: number,
  canvasHeight: number,
): void {
  expect(placement.x - LEAF_WIDTH / 2).toBeGreaterThanOrEqual(0)
  expect(placement.x + LEAF_WIDTH / 2).toBeLessThanOrEqual(canvasWidth)
  expect(placement.y - LEAF_HEIGHT / 2).toBeGreaterThanOrEqual(0)
  expect(placement.y + LEAF_HEIGHT / 2).toBeLessThanOrEqual(canvasHeight)
}

function problems(count: number): LearningProblemSummary[] {
  return Array.from({ length: count }, (_, index) => ({
    problemId: index + 1,
    title: `题目 ${index + 1}`,
    slug: `problem-${index + 1}`,
    difficulty: 2,
    solved: index % 2 === 0,
  }))
}
