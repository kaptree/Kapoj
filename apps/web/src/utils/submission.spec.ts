import { describe, expect, it } from 'vitest'

import {
  languageLabel,
  monacoLanguage,
  statusLabel,
  statusTagType,
  SUBMISSION_LANGUAGE_FILTERS,
  SUBMISSION_LANGUAGES,
  SUBMISSION_STATUSES,
} from './submission'

describe('submission utils', () => {
  it('maps every status to a label and tag type', () => {
    for (const status of SUBMISSION_STATUSES) {
      expect(statusLabel(status.value)).toBe(status.label)
      expect(statusTagType(status.value)).toBeTruthy()
    }
    expect(statusLabel('accepted')).toBe('通过')
    expect(statusTagType('accepted')).toBe('success')
    expect(statusTagType('wrong_answer')).toBe('danger')
    expect(statusTagType('time_limit_exceeded')).toBe('warning')
  })

  it('maps languages to labels and Monaco ids', () => {
    for (const language of SUBMISSION_LANGUAGES) {
      expect(languageLabel(language.value)).toBe(language.label)
    }
    expect(languageLabel('cpp')).toBe('C++')
    expect(monacoLanguage('cpp')).toBe('cpp')
    expect(monacoLanguage('c')).toBe('cpp')
    expect(monacoLanguage('python3')).toBe('python')
    expect(monacoLanguage('java')).toBe('java')
  })

  it('labels objective answers without offering answer as a code language', () => {
    expect(languageLabel('answer')).toBe('客观题')
    expect(SUBMISSION_LANGUAGES.map((language) => language.value)).not.toContain('answer')
    expect(SUBMISSION_LANGUAGE_FILTERS[0]).toEqual({ value: 'answer', label: '客观题' })
    expect(SUBMISSION_LANGUAGE_FILTERS.map((language) => language.value)).toEqual([
      'answer',
      ...SUBMISSION_LANGUAGES.map((language) => language.value),
    ])
  })
})
