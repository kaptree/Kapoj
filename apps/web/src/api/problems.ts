import { http } from './client'
import type {
  HydroImportResult,
  OfficialSolutionInput,
  OfficialSolutionView,
  Page,
  ProblemDetail,
  ProblemInput,
  ProblemListItem,
  ProblemStats,
  ProblemUpdateInput,
  Testcase,
  TestcaseGenerationConfirm,
  TestcaseGenerationInput,
  TestcaseGenerationPreview,
  TestcaseGenerationView,
} from './types'

export interface ListProblemsQuery {
  keyword?: string
  tag?: string
  difficulty?: number | null
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

export const getOfficialSolution = (id: number | string) =>
  http.get<OfficialSolutionView>(`/problems/${id}/official-solution`)

export const updateOfficialSolution = (id: number | string, input: OfficialSolutionInput) =>
  http.put<OfficialSolutionView>(`/problems/${id}/official-solution`, input)

export const purchaseOfficialSolution = (id: number | string) =>
  http.post<OfficialSolutionView>(`/problems/${id}/official-solution/purchase`)

export const listTestcases = (id: number | string) =>
  http.get<Testcase[]>(`/problems/${id}/testcases`)

export interface TestcaseUploadInput {
  input: File
  output: File
  score?: number
  isSample?: boolean
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

export function importHydroProblems(file: File) {
  const form = new FormData()
  form.append('file', file)
  return http.postForm<HydroImportResult>('/problems/import/hydro', form)
}

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
