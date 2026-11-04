import { http } from './client'
import type { CreateSubmissionRequest, SubmissionCreated } from './types'

export const createSubmission = (input: CreateSubmissionRequest) =>
  http.post<SubmissionCreated>('/submissions', input)
