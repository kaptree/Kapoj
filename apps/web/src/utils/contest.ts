import type { ContestMode, ContestRegisterType, ContestStatus } from '@/api/types'

export const CONTEST_STATUSES: { value: ContestStatus; label: string }[] = [
  { value: 'upcoming', label: '未开始' },
  { value: 'running', label: '进行中' },
  { value: 'ended', label: '已结束' },
]

export const CONTEST_MODES: { value: ContestMode; label: string }[] = [
  { value: 'icpc', label: 'ICPC' },
  { value: 'ioi', label: 'IOI' },
  { value: 'oi', label: 'OI' },
]

export const CONTEST_REGISTER_TYPES: { value: ContestRegisterType; label: string }[] = [
  { value: 'open', label: '公开报名' },
  { value: 'password', label: '密码报名' },
  { value: 'invite', label: '邀请码报名' },
]

const statusLabels = new Map(CONTEST_STATUSES.map((s) => [s.value, s.label]))
const modeLabels = new Map(CONTEST_MODES.map((m) => [m.value, m.label]))
const registerLabels = new Map(CONTEST_REGISTER_TYPES.map((r) => [r.value, r.label]))

export function contestStatusLabel(status: ContestStatus): string {
  return statusLabels.get(status) ?? status
}

export function contestStatusTagType(
  status: ContestStatus,
): 'success' | 'warning' | 'danger' | 'info' | 'primary' {
  if (status === 'running') return 'success'
  if (status === 'upcoming') return 'warning'
  return 'info'
}

export function contestModeLabel(mode: ContestMode): string {
  return modeLabels.get(mode) ?? mode
}

export function registerTypeLabel(type: ContestRegisterType): string {
  return registerLabels.get(type) ?? type
}

/** Formats a non-negative millisecond duration as "Xd HH:MM:SS". */
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '00:00:00'
  const total = Math.floor(ms / 1000)
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  const clock = [hours, minutes, seconds].map((n) => String(n).padStart(2, '0')).join(':')
  return days > 0 ? `${days}天 ${clock}` : clock
}

/**
 * Returns a countdown label using the server clock skew captured when the
 * contest was fetched. Returns null when the target time is invalid.
 */
export function contestCountdown(
  startTime: string,
  endTime: string,
  serverSkewMs: number,
  nowMs = Date.now(),
): { phase: 'upcoming' | 'running' | 'ended'; text: string } | null {
  const start = new Date(startTime).getTime()
  const end = new Date(endTime).getTime()
  if (!Number.isFinite(start) || !Number.isFinite(end)) return null
  const serverNow = nowMs + (Number.isFinite(serverSkewMs) ? serverSkewMs : 0)
  if (serverNow < start) {
    return { phase: 'upcoming', text: `距开始 ${formatDuration(start - serverNow)}` }
  }
  if (serverNow < end) {
    return { phase: 'running', text: `距结束 ${formatDuration(end - serverNow)}` }
  }
  return { phase: 'ended', text: '已结束' }
}
