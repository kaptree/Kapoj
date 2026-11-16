import { http } from './client'
import type {
  ContestSimilarityMatch,
  ContestSimilarityMatchDetail,
  ContestSimilarityOverview,
  ContestSimilarityRun,
  ContestSimilarityRunList,
  ContestSimilaritySettings,
  ContestSimilaritySettingsInput,
  Page,
} from './types'

export interface ListSimilarityMatchesQuery {
  problemId?: number | string
  runId?: number | string
  page?: number
  pageSize?: number
}

export const getSimilarityOverview = (contestId: number | string) =>
  http.get<ContestSimilarityOverview>(`/contests/${contestId}/similarity/overview`)

export const updateSimilaritySettings = (
  contestId: number | string,
  problemId: number | string,
  input: ContestSimilaritySettingsInput,
) =>
  http.put<ContestSimilaritySettings>(
    `/contests/${contestId}/similarity/problems/${problemId}`,
    input,
  )

export const startSimilarityRuns = (contestId: number | string, problemId?: number | string) =>
  http.post<ContestSimilarityRunList>(
    `/contests/${contestId}/similarity/runs`,
    problemId ? { problemId } : undefined,
  )

export const getSimilarityRun = (contestId: number | string, runId: number | string) =>
  http.get<ContestSimilarityRun>(`/contests/${contestId}/similarity/runs/${runId}`)

export const listSimilarityMatches = (
  contestId: number | string,
  query: ListSimilarityMatchesQuery = {},
) =>
  http.get<Page<ContestSimilarityMatch>>(`/contests/${contestId}/similarity/matches`, {
    params: {
      problemId: query.problemId || undefined,
      runId: query.runId || undefined,
      page: query.page || undefined,
      page_size: query.pageSize || undefined,
    },
  })

export const getSimilarityMatch = (contestId: number | string, matchId: number | string) =>
  http.get<ContestSimilarityMatchDetail>(`/contests/${contestId}/similarity/matches/${matchId}`)

export const downloadSimilarityCSV = (contestId: number | string, problemId?: number | string) =>
  http.getBlob(`/contests/${contestId}/similarity/export.csv`, {
    params: { problemId: problemId || undefined },
  })
