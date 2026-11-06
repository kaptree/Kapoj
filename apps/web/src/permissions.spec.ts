import { describe, expect, it } from 'vitest'

import {
  ADMIN_ACCESS_PERMISSIONS,
  CONTEST_MANAGE_PERMISSIONS,
  GAMIFICATION_MANAGE_PERMISSIONS,
  PERM,
  PROBLEM_MANAGE_PERMISSIONS,
} from './permissions'

describe('permission catalog', () => {
  it('defines the four gamification manage codes', () => {
    expect(GAMIFICATION_MANAGE_PERMISSIONS).toEqual([
      'gamification.achievement.manage',
      'gamification.task.manage',
      'gamification.badge.manage',
      'gamification.title.manage',
    ])
  })

  it('defines the problem manage codes', () => {
    expect(PROBLEM_MANAGE_PERMISSIONS).toEqual([
      'problem.view_hidden',
      'problem.create',
      'problem.update',
      'problem.delete',
      'problem.publish',
      'problem.testcase.manage',
      'problem.checker.manage',
    ])
  })

  it('defines the contest manage codes', () => {
    expect(CONTEST_MANAGE_PERMISSIONS).toEqual([
      'contest.view_hidden',
      'contest.create',
      'contest.update',
      'contest.delete',
      'contest.participant.read',
      'contest.announcement.manage',
    ])
  })

  it('ADMIN_ACCESS_PERMISSIONS covers every management domain', () => {
    for (const code of [
      'admin.dashboard.view',
      'admin.audit.view',
      'system.settings.manage',
      'user.read',
      'user.update',
      'user.ban',
      'user.coins.adjust',
      'user.roles.assign',
      'role.read',
      'role.create',
      'role.update',
      'role.delete',
      'role.permissions.update',
      'submission.view_live_contest',
      'gamification.config.manage',
      'learning_path.official.manage',
      'learning_path.admin.read',
    ]) {
      expect(ADMIN_ACCESS_PERMISSIONS).toContain(code)
    }
    for (const code of [
      ...PROBLEM_MANAGE_PERMISSIONS,
      ...CONTEST_MANAGE_PERMISSIONS,
      ...GAMIFICATION_MANAGE_PERMISSIONS,
    ]) {
      expect(ADMIN_ACCESS_PERMISSIONS).toContain(code)
    }
  })

  it('PERM exposes the stable codes used by the UI', () => {
    expect(PERM.problem.viewHidden).toBe('problem.view_hidden')
    expect(PERM.problem.create).toBe('problem.create')
    expect(PERM.problem.update).toBe('problem.update')
    expect(PERM.problem.delete).toBe('problem.delete')
    expect(PERM.contest.viewHidden).toBe('contest.view_hidden')
    expect(PERM.contest.create).toBe('contest.create')
    expect(PERM.contest.update).toBe('contest.update')
    expect(PERM.contest.delete).toBe('contest.delete')
    expect(PERM.contest.participant.read).toBe('contest.participant.read')
    expect(PERM.contest.announcement.manage).toBe('contest.announcement.manage')
    expect(PERM.gamification.achievement.manage).toBe('gamification.achievement.manage')
  })
})
