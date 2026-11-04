import type { components } from './schema'

type Schemas = components['schemas']

export type Me = Schemas['Me']
export type Profile = Schemas['Profile']
export type Role = Schemas['Role']
export type TokenPair = Schemas['TokenPair']
export type Session = Schemas['Session']
export type RegisterRequest = Schemas['RegisterRequest']
export type LoginRequest = Schemas['LoginRequest']
export type UpdateMeRequest = Schemas['UpdateMeRequest']
export type ProblemType = Schemas['ProblemType']
export type ProblemInput = Schemas['ProblemInput']
export type ProblemUpdateInput = Schemas['ProblemUpdateInput']
export type ProblemListItem = Schemas['ProblemListItem']
export type ProblemDetail = Schemas['ProblemDetail']
export type ProblemStats = Schemas['Stats']
export type Sample = Schemas['Sample']
export type Testcase = Schemas['Testcase']
export type SubmissionLanguage = Schemas['SubmissionLanguage']
export type SubmissionStatus = Schemas['SubmissionStatus']
export type CreateSubmissionRequest = Schemas['CreateSubmissionRequest']
export type SubmissionCreated = Schemas['SubmissionCreated']
export type SubmissionJudgeCase = Schemas['SubmissionJudgeCase']
export type SubmissionListItem = Schemas['SubmissionListItem']
export type SubmissionDetail = Schemas['SubmissionDetail']

export interface Page<T> {
  total: number
  items: T[]
}
