import type { LearningNodeStatus, LearningPathItem, LearningPathNodeItem } from '@/api/types'

export function learningStatusLabel(status: LearningNodeStatus): string {
  if (status === 'completed') return '已完成'
  if (status === 'active') return '进行中'
  return '未解锁'
}

export function learningStatusSeverity(
  status: LearningNodeStatus,
): 'success' | 'info' | 'secondary' {
  if (status === 'completed') return 'success'
  if (status === 'active') return 'info'
  return 'secondary'
}

export function learningPathVisibilityLabel(item: LearningPathItem): string {
  if (item.isOfficial) return '官方'
  if (item.isPublic) return '公开'
  return '私有'
}

export function nodeProgressPercent(solved: number, required: number): number {
  if (required <= 0) return 0
  if (solved <= 0) return 0
  return Math.min(100, Math.round((solved / required) * 100))
}

/** Pre-order flattening of a node forest into a single array. */
export function flattenLearningNodes(nodes: LearningPathNodeItem[]): LearningPathNodeItem[] {
  const result: LearningPathNodeItem[] = []
  for (const node of nodes) {
    result.push(node)
    result.push(...flattenLearningNodes(node.children))
  }
  return result
}

/** Depth-first search for a node by id across the whole forest. */
export function findLearningNode(
  nodes: LearningPathNodeItem[],
  id: number,
): LearningPathNodeItem | undefined {
  for (const node of nodes) {
    if (node.id === id) return node
    const found = findLearningNode(node.children, id)
    if (found) return found
  }
  return undefined
}

/** The set of ids for the node itself plus all of its descendants. */
export function descendantNodeIds(node: LearningPathNodeItem): Set<number> {
  const ids = new Set<number>([node.id])
  for (const child of node.children) {
    for (const id of descendantNodeIds(child)) ids.add(id)
  }
  return ids
}

/**
 * Nodes sharing a parent, sorted by `sortOrder` then `id`, mirroring the
 * server's stable ordering. The `parentId` matches either a number or null
 * (root level).
 */
export function siblingNodes(
  nodes: LearningPathNodeItem[],
  parentId: number | null,
): LearningPathNodeItem[] {
  return nodes
    .filter((node) => node.parentId === parentId)
    .slice()
    .sort((a, b) => a.sortOrder - b.sortOrder || a.id - b.id)
}
