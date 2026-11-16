import { describe, expect, it } from 'vitest'
import { COMPANION_PAGE_NOTES } from './pageNotes'

const expected = [
  'problems', 'problem-detail', 'problem-submit', 'problem-official-solution',
  'problem-solutions', 'problem-solution-detail', 'problem-statistics',
  'problem-contests', 'contests', 'contest-detail', 'submissions', 'leaderboard',
  'learning-paths', 'learning-path-detail', 'tasks', 'achievements', 'badge-shop',
  'title-shop', 'notifications', 'certificate-verify', 'certificate-settings',
  'user-profile', 'appearance',
]

const excluded = [
  'home', 'login', 'register', 'forgot-password', 'reset-password', 'verify-email',
  'profile-edit', 'security-settings', 'ai-settings', 'problem-create', 'problem-edit',
  'problem-import', 'problem-testcase-generator', 'contest-create', 'contest-edit',
  'learning-path-create', 'learning-path-edit', 'forbidden', 'not-found', 'admin-dashboard',
]

describe('companion page note map', () => {
  it('covers every B-tier route', () => {
    for (const route of expected) expect(COMPANION_PAGE_NOTES[route]).toBeTruthy()
  })

  it('leaves A/C/admin/auth routes out', () => {
    for (const route of excluded) expect(COMPANION_PAGE_NOTES[route]).toBeUndefined()
  })
})
