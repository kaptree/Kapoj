import type { SubmissionLanguage, SubmissionStatus } from '@/api/types'

export const SUBMISSION_STATUSES: { value: SubmissionStatus; label: string }[] = [
  { value: 'pending', label: '等待评测' },
  { value: 'judging', label: '评测中' },
  { value: 'accepted', label: '通过' },
  { value: 'wrong_answer', label: '答案错误' },
  { value: 'time_limit_exceeded', label: '超时' },
  { value: 'memory_limit_exceeded', label: '内存超限' },
  { value: 'runtime_error', label: '运行错误' },
  { value: 'compile_error', label: '编译错误' },
  { value: 'system_error', label: '系统错误' },
]

/** Submission languages backed by a code editor or compiler. */
export type CodeSubmissionLanguage = Exclude<SubmissionLanguage, 'answer'>

export const SUBMISSION_LANGUAGES: { value: CodeSubmissionLanguage; label: string }[] = [
  { value: 'c', label: 'C' },
  { value: 'cpp', label: 'C++' },
  { value: 'python3', label: 'Python 3' },
  { value: 'java', label: 'Java' },
  { value: 'go', label: 'Go' },
]

/** Language filter options include objective answers, which are not code languages. */
export const SUBMISSION_LANGUAGE_FILTERS: { value: SubmissionLanguage; label: string }[] = [
  { value: 'answer', label: '客观题' },
  ...SUBMISSION_LANGUAGES,
]

const statusLabels = new Map(SUBMISSION_STATUSES.map((s) => [s.value, s.label]))
const languageLabels = new Map(SUBMISSION_LANGUAGE_FILTERS.map((l) => [l.value, l.label]))

export function statusLabel(status: SubmissionStatus): string {
  return statusLabels.get(status) ?? status
}

export function statusTagType(
  status: SubmissionStatus,
): 'success' | 'warning' | 'danger' | 'info' | 'primary' {
  switch (status) {
    case 'accepted':
      return 'success'
    case 'wrong_answer':
    case 'runtime_error':
    case 'system_error':
      return 'danger'
    case 'time_limit_exceeded':
    case 'memory_limit_exceeded':
      return 'warning'
    case 'judging':
      return 'primary'
    default:
      return 'info'
  }
}

export function languageLabel(language: SubmissionLanguage): string {
  return languageLabels.get(language) ?? language
}

/** Maps a submission language to the editor language id. */
export function editorLanguage(language: SubmissionLanguage): string {
  if (language === 'cpp' || language === 'c') return 'cpp'
  if (language === 'python3') return 'python'
  return language
}
