import { http } from './client'
import type {
  AIConfigView,
  AICompleteInput,
  AICompleteOutput,
  AIFormatInput,
  AIFormatOutput,
  AIHintOutput,
  AIModelTestResult,
  AIOptimizeInput,
  AIOptimizeOutput,
  AISaveConfigInput,
  AIUserModelInput,
  AIUserModelsView,
  AIUserModelView,
} from './types'

export const getAIConfig = () => http.get<AIConfigView>('/ai/configs')

export const saveAIConfig = (input: AISaveConfigInput) =>
  http.post<AIConfigView>('/ai/configs', input)

export const listAIUserModels = () => http.get<AIUserModelsView>('/ai/models')

export const createAIUserModel = (input: AIUserModelInput) =>
  http.post<AIUserModelView>('/ai/models', input)

export const updateAIUserModel = (id: number | string, input: AIUserModelInput) =>
  http.put<AIUserModelView>(`/ai/models/${id}`, input)

export const deleteAIUserModel = (id: number | string) => http.delete<void>(`/ai/models/${id}`)

export const setActiveAIUserModel = (id: number | string) =>
  http.put<AIUserModelView>(`/ai/models/${id}/active`)

export const testAIUserModel = (id: number | string) =>
  http.post<AIModelTestResult>(`/ai/models/${id}/test`)

export const completeCode = (input: AICompleteInput) =>
  http.post<AICompleteOutput>('/ai/complete', input)

export const formatCode = (input: AIFormatInput) => http.post<AIFormatOutput>('/ai/format', input)

export const optimizeCode = (input: AIOptimizeInput) =>
  http.post<AIOptimizeOutput>('/ai/optimize', input)

export const getAIHint = (problemId: number) => http.post<AIHintOutput>('/ai/hint', { problemId })
