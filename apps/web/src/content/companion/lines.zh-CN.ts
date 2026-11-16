export type CompanionLineKey =
  | 'greeting'
  | 'signin'
  | 'ac'
  | 'wa'
  | 'ce'
  | 'tle'
  | 're'
  | 'empty'
  | 'lost'
  | 'note.problems'
  | 'note.problem'
  | 'note.submit'
  | 'note.solutions'
  | 'note.contests'
  | 'note.contest'
  | 'note.submissions'
  | 'note.leaderboard'
  | 'note.learning'
  | 'note.tasks'
  | 'note.achievements'
  | 'note.shop'
  | 'note.notifications'
  | 'note.certificate'
  | 'note.profile'
  | 'note.appearance'
  | 'note.statistics'

/**
 * Companion copy. Every line is Chinese, tsundere but never hostile, and short
 * enough for the speech bubble; failures always point at a next action.
 */
export const COMPANION_LINES: Record<CompanionLineKey, readonly string[]> = {
  greeting: ['又来了？……题可不会自己变简单。', '别误会，我只是刚好在看你提交。'],
  signin: ['签到了。今天也别让我记太多 WA。', '嗯，签到。……别以为我在等你。'],
  ac: ['过了。下一题，别得意。', '……不错。我说了不错，别让我重复。'],
  wa: ['第 3 个点。别看我，看代码。', '又 WA 了？……再读一遍题面。'],
  ce: ['编译器都看不下去了。', '先让它编译通过。……我等你。'],
  tle: ['跑得比我还慢。', '超时了。……优化一下。'],
  re: ['数组越界？……真拿你没办法。', '运行时崩了。检查边界。'],
  empty: ['这里空空的。……要一起填满吗？'],
  lost: ['这里什么都没有。……跟着我。'],
  'note.problems': ['今天的题……自己挑。别贪多。'],
  'note.problem': ['先读题。别急着写。'],
  'note.submit': ['代码写完再看一眼。……我可不帮你 debug。'],
  'note.solutions': ['看题解可以，别直接抄。'],
  'note.contests': ['比赛很多。选一个，认真打。'],
  'note.contest': ['开始了就专注点。……我在外面等。'],
  'note.submissions': ['记录都在这里。……别翻太多。'],
  'note.leaderboard': ['想上榜？先 AC 一道。'],
  'note.learning': ['路线在这。走不走随你。'],
  'note.tasks': ['任务刷新了。做不做随你。'],
  'note.achievements': ['还差几个。……我记着。'],
  'note.shop': ['看看就好，别乱花。'],
  'note.notifications': ['有人找你。……去看。'],
  'note.certificate': ['证书这东西……自己收好。'],
  'note.profile': ['你的主页。……还行。'],
  'note.appearance': ['换装？……先把我放出来。'],
  'note.statistics': ['数据不会骗人。……你自己看。'],
}

/** Small deterministic string hash so the same day picks the same line. */
function hashSeed(input: string): number {
  let hash = 0
  for (const char of input) {
    hash = (hash * 31 + char.codePointAt(0)!) >>> 0
  }
  return hash
}

export function pickCompanionLine(key: CompanionLineKey, seed = ''): string {
  const lines = COMPANION_LINES[key]
  return lines[hashSeed(`${key}:${seed}`) % lines.length]
}
