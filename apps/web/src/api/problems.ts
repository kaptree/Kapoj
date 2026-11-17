import { http } from './client'
import type {
  BulkProblemActionData,
  BulkProblemActionRequest,
  HydroImportResult,
  OfficialSolutionInput,
  OnlineTestcaseInput,
  OfficialSolutionView,
  Page,
  ProblemAttachment,
  ProblemDetail,
  ProblemImportResult,
  ProblemInput,
  ProblemListItem,
  ProblemPackageInfo,
  ProblemStats,
  ProblemSubmissionMetric,
  ProblemUpdateInput,
  StatementImageResult,
  Testcase,
  TestcaseContent,
  TestcaseContentInput,
  TestcaseGenerationConfirm,
  TestcaseGenerationInput,
  TestcaseGenerationPreview,
  TestcaseGenerationView,
} from './types'

export interface ListProblemsQuery {
  keyword?: string
  tag?: string
  difficulty?: number | null
  scope?: 'public' | 'mine' | 'all'
  includeHidden?: boolean
  page?: number
  pageSize?: number
}

export const listProblems = (q: ListProblemsQuery = {}) =>
  http.get<Page<ProblemListItem>>('/problems', {
    params: {
      keyword: q.keyword || undefined,
      tag: q.tag || undefined,
      difficulty: q.difficulty ?? undefined,
      scope: q.scope,
      includeHidden: q.includeHidden ? 'true' : undefined,
      page: q.page,
      page_size: q.pageSize,
    },
  })

export const getProblem = (id: number | string) => http.get<ProblemDetail>(`/problems/${id}`)

export const createProblem = (input: ProblemInput) => http.post<ProblemDetail>('/problems', input)

export const updateProblem = (id: number | string, input: ProblemUpdateInput) =>
  http.put<ProblemDetail>(`/problems/${id}`, input)

export const deleteProblem = (id: number | string) =>
  http.delete<{ offlined: boolean }>(`/problems/${id}`)

export const getStatistics = (id: number | string) =>
  http.get<ProblemStats>(`/problems/${id}/statistics`)

export type SubmissionMetricSort = 'time' | 'memory' | 'code_length' | 'created_at'

export interface SubmissionStatsQuery {
  sort?: SubmissionMetricSort
  order?: 'asc' | 'desc'
  page?: number
  pageSize?: number
}

export const getSubmissionStats = (id: number | string, q: SubmissionStatsQuery = {}) =>
  http.get<Page<ProblemSubmissionMetric>>(`/problems/${id}/submission-stats`, {
    params: {
      sort: q.sort,
      order: q.order,
      page: q.page,
      page_size: q.pageSize,
    },
  })

export const listProblemAttachments = (id: number | string) =>
  http.get<ProblemAttachment[]>(`/problems/${id}/attachments`)

export function uploadProblemAttachment(id: number | string, file: File) {
  const form = new FormData()
  form.append('file', file)
  return http.postForm<ProblemAttachment>(`/problems/${id}/attachments`, form)
}

export const deleteProblemAttachment = (id: number | string, attachmentId: number | string) =>
  http.delete<null>(`/problems/${id}/attachments/${attachmentId}`)

export function uploadStatementImage(id: number | string, file: File) {
  const form = new FormData()
  form.append('file', file)
  return http.postForm<StatementImageResult>(`/problems/${id}/assets`, form)
}

export const getOfficialSolution = (id: number | string) =>
  http.get<OfficialSolutionView>(`/problems/${id}/official-solution`)

export const updateOfficialSolution = (id: number | string, input: OfficialSolutionInput) =>
  http.put<OfficialSolutionView>(`/problems/${id}/official-solution`, input)

export const purchaseOfficialSolution = (id: number | string) =>
  http.post<OfficialSolutionView>(`/problems/${id}/official-solution/purchase`)

export const listTestcases = (id: number | string) =>
  http.get<Testcase[]>(`/problems/${id}/testcases`)

export const getTestcaseContent = (id: number | string, testcaseId: number | string) =>
  http.get<TestcaseContent>(`/problems/${id}/testcases/${testcaseId}/content`)

export const updateTestcaseContent = (
  id: number | string,
  testcaseId: number | string,
  input: TestcaseContentInput,
) => http.put<null>(`/problems/${id}/testcases/${testcaseId}/content`, input)

export const createOnlineTestcase = (id: number | string, input: OnlineTestcaseInput) =>
  http.post<Testcase>(`/problems/${id}/testcases/online`, input)

export interface TestcaseUploadInput {
  input: File
  output: File
  score?: number
  isSample?: boolean
}

export function uploadTestcasesBatch(id: number | string, files: File[], score = 10) {
  const form = new FormData()
  for (const file of files) form.append('files', file)
  form.append('score', String(score))
  return http.postForm<Testcase[]>(`/problems/${id}/testcases/batch`, form)
}

export function uploadTestcase(id: number | string, up: TestcaseUploadInput) {
  const form = new FormData()
  form.append('input', up.input)
  form.append('output', up.output)
  if (up.score !== undefined) form.append('score', String(up.score))
  if (up.isSample !== undefined) form.append('isSample', String(up.isSample))
  return http.postForm<Testcase>(`/problems/${id}/testcases`, form)
}

export const updateTestcase = (
  id: number | string,
  tid: number,
  changes: { score?: number; isSample?: boolean },
) => http.put<null>(`/problems/${id}/testcases/${tid}`, changes)

export const reorderTestcases = (id: number | string, ids: number[]) =>
  http.post<null>(`/problems/${id}/testcases/reorder`, { ids })

export const deleteTestcase = (id: number | string, tid: number) =>
  http.delete<null>(`/problems/${id}/testcases/${tid}`)

export function uploadChecker(id: number | string, file: File) {
  const form = new FormData()
  form.append('file', file)
  return http.postForm<null>(`/problems/${id}/checker`, form)
}

export interface ProblemImportOptions {
  conflictPolicy?: 'ask' | 'skip' | 'overwrite'
  overwriteSlugs?: string[]
  skipSlugs?: string[]
  publish?: boolean
}

/** @deprecated use ProblemImportOptions */
export type HydroImportOptions = ProblemImportOptions

function problemImportForm(file: File, options: ProblemImportOptions): FormData {
  const form = new FormData()
  form.append('file', file)
  if (options.conflictPolicy) form.append('conflictPolicy', options.conflictPolicy)
  if (options.overwriteSlugs?.length) {
    form.append('overwriteSlugs', JSON.stringify(options.overwriteSlugs))
  }
  if (options.skipSlugs?.length) form.append('skipSlugs', JSON.stringify(options.skipSlugs))
  if (options.publish) form.append('publish', 'true')
  return form
}

export function importHydroProblems(file: File, options: ProblemImportOptions = {}) {
  return http.postForm<HydroImportResult>(
    '/problems/import/hydro',
    problemImportForm(file, options),
  )
}

/** Imports a kapOJ single/batch package or a HydroOJ package (auto-detected). */
export function importProblemPackage(file: File, options: ProblemImportOptions = {}) {
  return http.postForm<ProblemImportResult>(
    '/problems/import/package',
    problemImportForm(file, options),
  )
}

export const getProblemPackageInfo = (id: number | string) =>
  http.get<ProblemPackageInfo>(`/problems/${id}/package/info`)

export const exportProblemPackage = (id: number | string) => http.getBlob(`/problems/${id}/package`)

export const bulkProblemAction = (input: BulkProblemActionRequest) =>
  http.post<BulkProblemActionData>('/problems/bulk/actions', input)

export const exportProblems = (problemIds: number[]) =>
  http.postBlob('/problems/bulk/export', { problemIds })

export const createTestcaseGeneration = (id: number | string, input: TestcaseGenerationInput) =>
  http.post<TestcaseGenerationView>(`/problems/${id}/testcase-generations`, input)

export const getTestcaseGeneration = (id: number | string, genId: number | string) =>
  http.get<TestcaseGenerationView>(`/problems/${id}/testcase-generations/${genId}`)

export const previewTestcaseGeneration = (
  id: number | string,
  genId: number | string,
  caseId: number | string,
  kind: 'input' | 'output',
) =>
  http.get<TestcaseGenerationPreview>(
    `/problems/${id}/testcase-generations/${genId}/cases/${caseId}/preview`,
    { params: { kind } },
  )

export const confirmTestcaseGeneration = (id: number | string, genId: number | string) =>
  http.post<TestcaseGenerationConfirm>(`/problems/${id}/testcase-generations/${genId}/confirm`)

export const discardTestcaseGeneration = (id: number | string, genId: number | string) =>
  http.delete<null>(`/problems/${id}/testcase-generations/${genId}`)
