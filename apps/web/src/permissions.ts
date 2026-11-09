/**
 * Stable frontend permission catalog.
 *
 * Permission codes use the `domain.action` format and must never be renamed.
 * They mirror the backend permission directory (see the admin RBAC design doc,
 * section 5) and drive every frontend authorization decision. Legacy
 * `user.role` must never be used for authorization.
 */
export const PERM = {
  admin: {
    dashboard: { view: 'admin.dashboard.view' },
    audit: { view: 'admin.audit.view' },
  },
  system: {
    settings: { manage: 'system.settings.manage' },
  },
  user: {
    read: 'user.read',
    update: 'user.update',
    ban: 'user.ban',
    coins: { adjust: 'user.coins.adjust' },
    roles: { assign: 'user.roles.assign' },
  },
  role: {
    read: 'role.read',
    create: 'role.create',
    update: 'role.update',
    delete: 'role.delete',
    permissions: { update: 'role.permissions.update' },
  },
  problem: {
    viewHidden: 'problem.view_hidden',
    create: 'problem.create',
    update: 'problem.update',
    delete: 'problem.delete',
    publish: 'problem.publish',
    testcase: { manage: 'problem.testcase.manage' },
    checker: { manage: 'problem.checker.manage' },
  },
  submission: {
    viewLiveContest: 'submission.view_live_contest',
  },
  contest: {
    viewHidden: 'contest.view_hidden',
    create: 'contest.create',
    update: 'contest.update',
    delete: 'contest.delete',
    participant: { read: 'contest.participant.read' },
    announcement: { manage: 'contest.announcement.manage' },
  },
  gamification: {
    achievement: { manage: 'gamification.achievement.manage' },
    task: { manage: 'gamification.task.manage' },
    badge: { manage: 'gamification.badge.manage' },
    title: { manage: 'gamification.title.manage' },
    config: { manage: 'gamification.config.manage' },
  },
  learningPath: {
    official: { manage: 'learning_path.official.manage' },
    admin: { read: 'learning_path.admin.read' },
  },
  rating: {
    settings: { manage: 'rating.settings.manage' },
  },
  ai: {
    settings: { manage: 'ai.settings.manage' },
  },
  certificate: {
    manage: 'certificate.manage',
  },
  site: {
    settings: { manage: 'site.settings.manage' },
  },
} as const

/** The four gamification domain manage permissions shown as admin tabs. */
export const GAMIFICATION_MANAGE_PERMISSIONS: string[] = [
  PERM.gamification.achievement.manage,
  PERM.gamification.task.manage,
  PERM.gamification.badge.manage,
  PERM.gamification.title.manage,
]

/** Every problem management permission code. */
export const PROBLEM_MANAGE_PERMISSIONS: string[] = [
  PERM.problem.viewHidden,
  PERM.problem.create,
  PERM.problem.update,
  PERM.problem.delete,
  PERM.problem.publish,
  PERM.problem.testcase.manage,
  PERM.problem.checker.manage,
]

/** Every contest management permission code. */
export const CONTEST_MANAGE_PERMISSIONS: string[] = [
  PERM.contest.viewHidden,
  PERM.contest.create,
  PERM.contest.update,
  PERM.contest.delete,
  PERM.contest.participant.read,
  PERM.contest.announcement.manage,
]

/**
 * Every permission code that grants access to some management surface
 * (admin shell, content, gamification and learning path). Used to decide
 * whether to show the administration entry point.
 */
export const ADMIN_ACCESS_PERMISSIONS: string[] = [
  PERM.admin.dashboard.view,
  PERM.admin.audit.view,
  PERM.system.settings.manage,
  PERM.user.read,
  PERM.user.update,
  PERM.user.ban,
  PERM.user.coins.adjust,
  PERM.user.roles.assign,
  PERM.role.read,
  PERM.role.create,
  PERM.role.update,
  PERM.role.delete,
  PERM.role.permissions.update,
  ...PROBLEM_MANAGE_PERMISSIONS,
  PERM.submission.viewLiveContest,
  ...CONTEST_MANAGE_PERMISSIONS,
  ...GAMIFICATION_MANAGE_PERMISSIONS,
  PERM.gamification.config.manage,
  PERM.learningPath.official.manage,
  PERM.learningPath.admin.read,
  PERM.rating.settings.manage,
  PERM.ai.settings.manage,
  PERM.certificate.manage,
  PERM.site.settings.manage,
]
