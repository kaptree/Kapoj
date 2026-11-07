import { describe, expect, it } from 'vitest'

import type { LearningPathItem, LearningPathNodeItem } from '@/api/types'
import {
  descendantNodeIds,
  findLearningNode,
  flattenLearningNodes,
  learningPathVisibilityLabel,
  learningStatusLabel,
  learningStatusSeverity,
  nodeProgressPercent,
  siblingNodes,
} from './learning'

describe('learning utils', () => {
  it('maps node status labels and severities', () => {
    expect(learningStatusLabel('completed')).toBe('已完成')
    expect(learningStatusLabel('active')).toBe('进行中')
    expect(learningStatusLabel('locked')).toBe('未解锁')

    expect(learningStatusSeverity('completed')).toBe('success')
    expect(learningStatusSeverity('active')).toBe('info')
    expect(learningStatusSeverity('locked')).toBe('secondary')
  })

  it('maps path visibility to 官方/公开/私有', () => {
    const base: LearningPathItem = {
      id: 1,
      title: '路线',
      descriptionMd: '',
      coverUrl: '',
      isOfficial: false,
      isPublic: false,
      isActive: true,
      author: { id: 1, username: 'a', nickname: 'A' },
      nodeCount: 1,
      myCompletedCount: 0,
      sortOrder: 0,
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    }

    expect(learningPathVisibilityLabel({ ...base, isOfficial: true, isPublic: true })).toBe('官方')
    expect(learningPathVisibilityLabel({ ...base, isOfficial: false, isPublic: true })).toBe('公开')
    expect(learningPathVisibilityLabel({ ...base, isOfficial: false, isPublic: false })).toBe(
      '私有',
    )
  })

  it('computes progress percent with a floor of 0', () => {
    expect(nodeProgressPercent(1, 3)).toBe(33)
    expect(nodeProgressPercent(3, 3)).toBe(100)
    expect(nodeProgressPercent(0, 3)).toBe(0)
    expect(nodeProgressPercent(5, 3)).toBe(100)
    expect(nodeProgressPercent(2, 0)).toBe(0)
    expect(nodeProgressPercent(2, -1)).toBe(0)
  })

  it('flattens nodes in pre-order', () => {
    const tree = [node(1, [node(2, [node(4)]), node(3)])]

    expect(flattenLearningNodes(tree).map((n) => n.id)).toEqual([1, 2, 4, 3])
  })

  it('finds a node by id anywhere in the tree', () => {
    const tree = [node(1, [node(2, [node(4)]), node(3)])]

    expect(findLearningNode(tree, 3)?.id).toBe(3)
    expect(findLearningNode(tree, 99)).toBeUndefined()
  })

  it('collects the node and all descendant ids', () => {
    const root = node(1, [node(2, [node(4)]), node(3)])

    expect(Array.from(descendantNodeIds(root)).sort((a, b) => a - b)).toEqual([1, 2, 3, 4])
  })

  it('returns siblings sorted by sortOrder then id', () => {
    const nodes = [
      node(30, [], 3, 7),
      node(10, [], 1, 7),
      node(20, [], 1, 7),
      node(15, [], 2, 7),
      node(99, [], 0, 8),
    ]

    expect(siblingNodes(nodes, 7).map((n) => n.id)).toEqual([10, 20, 15, 30])
  })
})

function node(
  id: number,
  children: LearningPathNodeItem[] = [],
  sortOrder = 0,
  parentId: number | null = null,
): LearningPathNodeItem {
  return {
    id,
    parentId,
    title: `节点 ${id}`,
    descriptionMd: '',
    problemIds: [],
    problems: [],
    sortOrder,
    requiredCount: 1,
    solvedCount: 0,
    status: 'locked',
    children,
  }
}
