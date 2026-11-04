import { http } from './client'
import type {
  CloneContestRequest,
  ContestDetail,
  ContestInput,
  ContestListItem,
  ContestMode,
  ContestParticipant,
  ContestStatus,
  Page,
} from './types'

export interface ListContestsQuery {
  keyword?: string
  mode?: ContestMode | ''
  status?: ContestStatus | ''
  includeHidden?: boolean
  page?: number
  pageSize?: number
}

export const listContests = (q: ListContestsQuery = {}) =>
  http.get<Page<ContestListItem>>('/contests', {
    params: {
      keyword: q.keyword || undefined,
      mode: q.mode || undefined,
      status: q.status || undefined,
      includeHidden: q.includeHidden ? 'true' : undefined,
      page: q.page,
      page_size: q.pageSize,
    },
  })

export const getContest = (id: number | string) => http.get<ContestDetail>(`/contests/${id}`)

export const createContest = (input: ContestInput) => http.post<ContestDetail>('/contests', input)

export const updateContest = (id: number | string, input: ContestInput) =>
  http.put<ContestDetail>(`/contests/${id}`, input)

export const deleteContest = (id: number | string) => http.delete<null>(`/contests/${id}`)

export const cloneContest = (id: number | string, input: CloneContestRequest) =>
  http.post<ContestDetail>(`/contests/${id}/clone`, input)

export const registerContest = (
  id: number | string,
  input: { password?: string; inviteCode?: string },
) => http.post<{ registered: boolean }>(`/contests/${id}/register`, input)

export const listContestParticipants = (id: number | string) =>
  http.get<ContestParticipant[]>(`/contests/${id}/participants`)
