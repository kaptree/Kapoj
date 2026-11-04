import { describe, expect, it } from 'vitest'

import {
  languageLabel,
  monacoLanguage,
  statusLabel,
  statusTagType,
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
})
