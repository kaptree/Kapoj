import type { CompanionLineKey } from './lines.zh-CN'
import type { CompanionExpression } from './types'

/**
 * One short in-page note per route name. Route names come from
 * `src/router/index.ts`; unnamed routes simply get no note.
 */
export const COMPANION_PAGE_NOTES: Record<
  string,
  { lineKey: CompanionLineKey; expression: CompanionExpression }
> = {
  problems: { lineKey: 'note.problems', expression: 'thinking' },
  'problem-detail': { lineKey: 'note.problem', expression: 'neutral' },
  'problem-submit': { lineKey: 'note.submit', expression: 'thinking' },
  'problem-official-solution': { lineKey: 'note.solutions', expression: 'smug' },
  'problem-solutions': { lineKey: 'note.solutions', expression: 'thinking' },
  'problem-solution-detail': { lineKey: 'note.solutions', expression: 'neutral' },
  'problem-statistics': { lineKey: 'note.statistics', expression: 'thinking' },
  'problem-contests': { lineKey: 'note.contests', expression: 'smug' },
  contests: { lineKey: 'note.contests', expression: 'smug' },
  'contest-detail': { lineKey: 'note.contest', expression: 'neutral' },
  submissions: { lineKey: 'note.submissions', expression: 'neutral' },
  leaderboard: { lineKey: 'note.leaderboard', expression: 'smug' },
  'learning-paths': { lineKey: 'note.learning', expression: 'thinking' },
  'learning-path-detail': { lineKey: 'note.learning', expression: 'neutral' },
  tasks: { lineKey: 'note.tasks', expression: 'thinking' },
  achievements: { lineKey: 'note.achievements', expression: 'smug' },
  'badge-shop': { lineKey: 'note.shop', expression: 'smug' },
  'title-shop': { lineKey: 'note.shop', expression: 'smug' },
  notifications: { lineKey: 'note.notifications', expression: 'surprised' },
  'certificate-verify': { lineKey: 'note.certificate', expression: 'neutral' },
  'certificate-settings': { lineKey: 'note.certificate', expression: 'neutral' },
  'user-profile': { lineKey: 'note.profile', expression: 'neutral' },
  appearance: { lineKey: 'note.appearance', expression: 'pout' },
}
