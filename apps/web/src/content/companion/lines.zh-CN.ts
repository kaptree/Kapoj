export type CompanionLineKey =
  | 'greeting'
  | 'greet.morning'
  | 'greet.afternoon'
  | 'greet.night'
  | 'idle.lively'
  | 'signin'
  | 'ac'
  | 'task'
  | 'achievement'
  | 'contest'
  | 'pet'
  | 'pet.happy'
  | 'pet.shy'
  | 'pet.pout'
  | 'pet.surprised'
  | 'pet.sleepy'
  | 'pet.smug'
  | 'pet.already'
  | 'streak'
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
  'greet.morning': [
    '早。……别以为我在等你。',
    '来了？先清醒一下。',
    '早上好。……今天也别让我等太久。',
  ],
  'greet.afternoon': ['下午了。题写完了吗？', '……午休够了就继续。', '这个点才来？……算了。'],
  'greet.night': [
    '这么晚还在？……注意休息。',
    '晚上好。……别熬太晚。',
    '夜里安静，适合写题。……也适合陪我。',
  ],
  'idle.lively': ['……你在忙什么？', '喂，别一直盯着屏幕。', '偶尔也看看我。……就一下。'],
  signin: ['签到了。今天也别让我记太多 WA。', '嗯，签到。……别以为我在等你。'],
  ac: ['过了。下一题，别得意。', '……不错。我说了不错，别让我重复。'],
  task: ['任务领了？那就做完。……别半途而废。', '任务不是摆设。去做。'],
  achievement: ['成就？……还行。别停下来。', '解锁了。……我看到了。'],
  contest: ['比赛开始了。……专心打。', '打完再来找我。……别分心。'],
  pet: ['……摸头也不行。就一下。', '别得寸进尺。……嗯。'],
  'pet.happy': [
    '嘴上不说……尾巴倒是摇起来了。',
    '摸一下就够了。……我没有开心。',
    '嗯……今天允许你多摸一下。',
  ],
  'pet.shy': ['别、别突然摸过来……', '就一下。再摸我要生气了。', '……脸有点热。别看了。'],
  'pet.pout': ['哼，才不是因为喜欢你。', '再摸就咬你。……真的。', '手拿开。……再放一会儿也行。'],
  'pet.surprised': ['哇……你走路没声音的吗？', '突然摸过来，吓我一跳。', '……下次先说一声。'],
  'pet.sleepy': ['困……别吵。', '再摸就睡着了。……嗯。', '让我靠一会儿。就一会儿。'],
  'pet.smug': ['手感不错吧？……我知道。', '哼，摸到宝了。', '这可是限定版。珍惜点。'],
  'pet.already': [
    '今天已经摸过了。……明天再来。',
    '又来？……今天额度用完了。',
    '贪心。……明天再给你摸。',
  ],
  streak: ['连续这么多天？……我记着呢。', '又来了。……习惯倒是不错。'],
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

export function pickCompanionLine(key: CompanionLineKey, seed = '', exclude = ''): string {
  const lines = COMPANION_LINES[key]
  const start = hashSeed(`${key}:${seed}`) % lines.length
  for (let offset = 0; offset < lines.length; offset += 1) {
    const line = lines[(start + offset) % lines.length]
    if (line !== exclude) return line
  }
  return lines[start]
}
