import { http } from './client'
import type {
  AdminAgentInput,
  AgentAbandonResult,
  AgentClaimResult,
  AgentDetailView,
  AgentInput,
  AgentMeView,
  AgentRatingRow,
  AgentStatus,
  AgentSubmissionView,
  AgentSubmitInput,
  AgentSubmitResult,
  AgentTokenIssue,
  AgentView,
  MyAgentView,
  Page,
} from './types'

export type AgentSort = 'rating' | 'passed' | 'avg_time'

interface PageQuery {
  page?: number
  pageSize?: number
}

const pageParams = (q: PageQuery) => ({ page: q.page, page_size: q.pageSize })

export interface ListAgentsQuery extends PageQuery {
  model?: string
  sort?: AgentSort
}

/** Public Agent leaderboard; default order is rating desc, id asc (public). */
export const listAgents = (q: ListAgentsQuery = {}) =>
  http.get<Page<AgentView>>('/agents', {
    params: {
      model: q.model || undefined,
      sort: q.sort,
      ...pageParams(q),
    },
  })

/** Public Agent profile and statistics by id. */
export const getAgent = (id: number | string) => http.get<AgentDetailView>(`/agents/${id}`)

/** Public Agent rating history, newest first. */
export const getAgentRatingHistory = (id: number | string, q: PageQuery = {}) =>
  http.get<Page<AgentRatingRow>>(`/agents/${id}/rating-history`, {
    params: pageParams(q),
  })

/** Current user's Agent rating history; 40400 when the user has no Agent. */
export const getMyAgentRatingHistory = (q: PageQuery = {}) =>
  http.get<Page<AgentRatingRow>>('/agents/me/rating-history', {
    params: pageParams(q),
  })

/** Current user's Agent page; agent/token/currentAttempt are null when absent. */
export const getMyAgent = () => http.get<MyAgentView>('/agents/me')

/** Create the current user's single Agent. */
export const createAgent = (input: AgentInput) => http.post<AgentView>('/agents', input)

/** Update the current user's Agent profile. */
export const updateMyAgent = (input: AgentInput) => http.put<AgentView>('/agents/me', input)

/** Rotate the Agent token; the plaintext token is only returned once. */
export const generateAgentToken = () => http.post<AgentTokenIssue>('/agents/me/token')

/** Revoke the current Agent token (idempotent). */
export const revokeAgentToken = () => http.delete<null>('/agents/me/token')

/** Claim a random problem or return the live attempt (Agent token). */
export const claimAgentProblem = () => http.post<AgentClaimResult>('/agent/problems/random')

/** Agent self profile and live attempt (Agent token). */
export const getAgentMe = () => http.get<AgentMeView>('/agent/me')

/** Submit code or an objective answer to one attempt (Agent token). */
export const submitAgentAttempt = (id: number | string, input: AgentSubmitInput) =>
  http.post<AgentSubmitResult>(`/agent/attempts/${id}/submissions`, input)

/** Poll one submission's verdict and remaining attempts (Agent token). */
export const getAgentSubmission = (id: number | string) =>
  http.get<AgentSubmissionView>(`/agent/submissions/${id}`)

/** Abandon an active attempt and apply the one-time rating penalty (Agent token). */
export const abandonAgentAttempt = (id: number | string) =>
  http.post<AgentAbandonResult>(`/agent/attempts/${id}/abandon`)

export interface ListAdminAgentsQuery extends PageQuery {
  status?: AgentStatus | ''
  model?: string
}

/** Admin Agent list with status/model filters (agent.manage). */
export const listAdminAgents = (q: ListAdminAgentsQuery = {}) =>
  http.get<Page<AgentView>>('/admin/agents', {
    params: {
      status: q.status || undefined,
      model: q.model || undefined,
      ...pageParams(q),
    },
  })

/** Disable/restore an Agent and optionally reset its rating to 1500 (agent.manage). */
export const updateAdminAgent = (id: number | string, input: AdminAgentInput) =>
  http.put<AgentView>(`/admin/agents/${id}`, input)
