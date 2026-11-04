export const DIFFICULTIES = [
  { value: 1, label: '入门' },
  { value: 2, label: '普及' },
  { value: 3, label: '提高' },
  { value: 4, label: '省选' },
  { value: 5, label: 'NOI' },
]

export function difficultyLabel(d: number): string {
  return DIFFICULTIES.find((x) => x.value === d)?.label ?? '未知'
}
