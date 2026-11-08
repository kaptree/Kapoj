import { http } from './client'
import type {
  AIConfigView,
  AICompleteInput,
  AICompleteOutput,
  AIFormatInput,
  AIFormatOutput,
  AIHintOutput,
  AIOptimizeInput,
  AIOptimizeOutput,
  AISaveConfigInput,
} from './types'

export const getAIConfig = () => http.get<AIConfigView>('/ai/configs')

export const saveAIConfig = (input: AISaveConfigInput) =>
  http.post<AIConfigView>('/ai/configs', input)

export const completeCode = (input: AICompleteInput) =>
  http.post<AICompleteOutput>('/ai/complete', input)

export const formatCode = (input: AIFormatInput) => http.post<AIFormatOutput>('/ai/format', input)

export const optimizeCode = (input: AIOptimizeInput) =>
  http.post<AIOptimizeOutput>('/ai/optimize', input)

export const getAIHint = (problemId: number) => http.post<AIHintOutput>('/ai/hint', { problemId })
