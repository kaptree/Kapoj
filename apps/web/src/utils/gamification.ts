import type { AchievementConditionType, TaskStatus, TitleUnlockType } from '@/api/types'

const ACHIEVEMENT_CONDITION_LABELS: Record<AchievementConditionType, string> = {
  ac_count: 'AC 题目数',
  contest_count: '参赛数',
  login_streak: '连续登录',
  created_contest: '创建比赛',
  all_of: '组合条件',
}

const TASK_STATUS_LABELS: Record<TaskStatus, string> = {
  upcoming: '未开始',
  active: '可领取',
  claimed: '进行中',
  completed: '已完成',
  expired: '已过期',
  ended: '已结束',
  inactive: '已下线',
}

const TASK_STATUS_TAG_TYPES: Record<
  TaskStatus,
  'success' | 'warning' | 'danger' | 'info' | 'primary'
> = {
  upcoming: 'info',
  active: 'success',
  claimed: 'warning',
  completed: 'success',
  expired: 'danger',
  ended: 'info',
  inactive: 'info',
}

const TITLE_UNLOCK_LABELS: Record<TitleUnlockType, string> = {
  rating: 'Rating',
  ac_count: 'AC 题目数',
  achievement: '成就',
}

const COIN_REASON_LABELS: Record<string, string> = {
  achievement: '成就奖励',
  task: '任务奖励',
  badge_purchase: '购买徽章',
  title_purchase: '购买称号',
  admin_adjust: '管理员调整',
}

export const DEFAULT_TITLE_COLOR = '#303133'

export function achievementConditionLabel(type: AchievementConditionType): string {
  return ACHIEVEMENT_CONDITION_LABELS[type] ?? type
}

export function taskStatusLabel(status: TaskStatus): string {
  return TASK_STATUS_LABELS[status] ?? status
}

export function taskStatusTagType(
  status: TaskStatus,
): 'success' | 'warning' | 'danger' | 'info' | 'primary' {
  return TASK_STATUS_TAG_TYPES[status] ?? 'info'
}

export function titleUnlockLabel(type: TitleUnlockType): string {
  return TITLE_UNLOCK_LABELS[type] ?? type
}

export function coinReasonLabel(reason: string): string {
  return COIN_REASON_LABELS[reason] ?? reason
}

/** Only accepts an exact #RRGGBB value, so untrusted colors cannot inject CSS. */
export function safeTitleColor(color: string): string {
  return /^#[0-9A-Fa-f]{6}$/.test(color) ? color : DEFAULT_TITLE_COLOR
}

/** Returns the first code point of a badge name, or ? when it is empty. */
export function badgeInitial(name: string): string {
  return Array.from(name.trim())[0] ?? '?'
}

/** Returns undefined for an empty image URL so el-avatar falls back to the initial. */
export function badgeImageSrc(url: string): string | undefined {
  return url.trim() || undefined
}

export function formatCoins(n: number): string {
  if (!Number.isFinite(n)) return '0'
  return Math.trunc(n).toLocaleString('zh-CN')
}
