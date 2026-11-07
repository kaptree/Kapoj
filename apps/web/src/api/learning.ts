import { http } from './client'
import type {
  LearningNodeInput,
  LearningPathInput,
  LearningPathItem,
  LearningPathNodeItem,
  LearningTree,
  Page,
} from './types'

export interface ListLearningPathsQuery {
  mine?: boolean
  includeInactive?: boolean
}

export const listLearningPaths = (q: ListLearningPathsQuery = {}) =>
  http.get<Page<LearningPathItem>>('/learning-paths', {
    params: {
      mine: q.mine ? 'true' : undefined,
      includeInactive: q.includeInactive ? 'true' : undefined,
    },
  })

export const getLearningPath = (id: number | string) =>
  http.get<LearningPathItem>(`/learning-paths/${id}`)

export const getLearningTree = (id: number | string) =>
  http.get<LearningTree>(`/learning-paths/${id}/tree`)

export const createLearningPath = (input: LearningPathInput) =>
  http.post<LearningPathItem>('/learning-paths', input)

export const updateLearningPath = (id: number | string, input: LearningPathInput) =>
  http.put<LearningPathItem>(`/learning-paths/${id}`, input)

export const deleteLearningPath = (id: number | string) =>
  http.delete<{ deleted: boolean }>(`/learning-paths/${id}`)

export const createLearningNode = (pathId: number | string, input: LearningNodeInput) =>
  http.post<LearningPathNodeItem>(`/learning-paths/${pathId}/nodes`, input)

export const updateLearningNode = (
  pathId: number | string,
  nodeId: number | string,
  input: LearningNodeInput,
) => http.put<LearningPathNodeItem>(`/learning-paths/${pathId}/nodes/${nodeId}`, input)

export const deleteLearningNode = (pathId: number | string, nodeId: number | string) =>
  http.delete<{ deleted: boolean }>(`/learning-paths/${pathId}/nodes/${nodeId}`)
