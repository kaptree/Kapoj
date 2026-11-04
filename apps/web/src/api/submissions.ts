import { http } from './client'
import type {
  CreateSubmissionRequest,
  Page,
  SubmissionCreated,
  SubmissionDetail,
  SubmissionLanguage,
  SubmissionListItem,
  SubmissionStatus,
} from './types'

export interface ListSubmissionsQuery {
  problemId?: number
  contestId?: number
  userId?: number
  language?: SubmissionLanguage | ''
  status?: SubmissionStatus | ''
  nickname?: string
  page?: number
  pageSize?: number
}

export const listSubmissions = (q: ListSubmissionsQuery = {}) =>
  http.get<Page<SubmissionListItem>>('/submissions', {
    params: {
      problemId: q.problemId || undefined,
      contestId: q.contestId === 0 ? 0 : q.contestId || undefined,
      userId: q.userId || undefined,
      language: q.language || undefined,
      status: q.status || undefined,
      nickname: q.nickname || undefined,
      page: q.page,
      page_size: q.pageSize,
    },
  })

export const getSubmission = (id: number | string) =>
  http.get<SubmissionDetail>(`/submissions/${id}`)

export const createSubmission = (input: CreateSubmissionRequest) =>
  http.post<SubmissionCreated>('/submissions', input)
