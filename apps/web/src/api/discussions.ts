import { http } from './client'
import type {
  Page,
  SolutionPostDetail,
  SolutionPostInput,
  SolutionPostItem,
  SolutionReplyInput,
  SolutionReplyItem,
} from './types'

export interface ListSolutionsQuery {
  page?: number
  pageSize?: number
}

export const listSolutionPosts = (id: number | string, q: ListSolutionsQuery = {}) =>
  http.get<Page<SolutionPostItem>>(`/problems/${id}/solutions`, {
    params: {
      page: q.page,
      page_size: q.pageSize,
    },
  })

export const createSolutionPost = (id: number | string, input: SolutionPostInput) =>
  http.post<SolutionPostDetail>(`/problems/${id}/solutions`, input)

export const getSolutionPost = (id: number | string, postId: number | string) =>
  http.get<SolutionPostDetail>(`/problems/${id}/solutions/${postId}`)

export const updateSolutionPost = (
  id: number | string,
  postId: number | string,
  input: SolutionPostInput,
) => http.put<SolutionPostDetail>(`/problems/${id}/solutions/${postId}`, input)

export const deleteSolutionPost = (id: number | string, postId: number | string) =>
  http.delete<null>(`/problems/${id}/solutions/${postId}`)

export const createSolutionReply = (
  id: number | string,
  postId: number | string,
  input: SolutionReplyInput,
) => http.post<SolutionReplyItem>(`/problems/${id}/solutions/${postId}/replies`, input)

export const updateSolutionReply = (
  id: number | string,
  postId: number | string,
  replyId: number | string,
  input: SolutionReplyInput,
) => http.put<SolutionReplyItem>(`/problems/${id}/solutions/${postId}/replies/${replyId}`, input)

export const deleteSolutionReply = (
  id: number | string,
  postId: number | string,
  replyId: number | string,
) => http.delete<null>(`/problems/${id}/solutions/${postId}/replies/${replyId}`)
