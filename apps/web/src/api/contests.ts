import { http } from './client'
import type {
  AnnouncementInput,
  AnnouncementItem,
  CloneContestRequest,
  ContestDetail,
  ContestInput,
  ContestListItem,
  ContestMode,
  ContestParticipant,
  ContestRanklist,
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

export const getContestRanklist = (id: number | string) =>
  http.get<ContestRanklist>(`/contests/${id}/ranklist`)

export const listAnnouncements = (id: number | string) =>
  http.get<AnnouncementItem[]>(`/contests/${id}/announcements`)

export const createAnnouncement = (id: number | string, input: AnnouncementInput) =>
  http.post<AnnouncementItem>(`/contests/${id}/announcements`, input)

export const updateAnnouncement = (
  id: number | string,
  aid: number | string,
  input: AnnouncementInput,
) => http.put<null>(`/contests/${id}/announcements/${aid}`, input)

export const deleteAnnouncement = (id: number | string, aid: number | string) =>
  http.delete<null>(`/contests/${id}/announcements/${aid}`)
