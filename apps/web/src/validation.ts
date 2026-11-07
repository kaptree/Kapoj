export const USERNAME_PATTERN = /^[a-zA-Z0-9_]{3,32}$/
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/** Number of UTF-8 bytes, mirroring the server's password length check. */
export function byteLength(value: string): number {
  return new TextEncoder().encode(value).length
}

/** Code-point count, matching the server's UTF-8 rune count for text fields. */
function characterCount(value: string): number {
  return Array.from(value).length
}

// These mirror the server rules in apps/server/internal/user/validate.go.
export function validateUsername(value: string): string | null {
  if (value === '') return '请输入用户名'
  if (!USERNAME_PATTERN.test(value)) return '用户名需为 3–32 位字母、数字或下划线'
  return null
}

export function validateEmail(value: string): string | null {
  if (value === '') return '请输入邮箱'
  if (value.length > 254 || !EMAIL_PATTERN.test(value)) return '邮箱格式不正确'
  return null
}

export function validatePassword(value: string): string | null {
  if (value === '') return '请输入密码'
  const bytes = byteLength(value)
  if (bytes < 8 || bytes > 72) return '密码长度需为 8–72 个字节'
  return null
}

export function validateLoginAccount(value: string): string | null {
  return value.trim() === '' ? '请输入用户名或邮箱' : null
}

/** Login only requires a non-empty password; legacy accounts may predate the rules. */
export function validateLoginPassword(value: string): string | null {
  return value === '' ? '请输入密码' : null
}

/** Registration nickname is optional; the server falls back to the username. */
export function validateNickname(value: string): string | null {
  return characterCount(value) > 32 ? '昵称不能超过 32 个字符' : null
}

export function validateProfileNickname(value: string): string | null {
  const trimmed = value.trim()
  if (trimmed === '') return '请输入昵称'
  if (characterCount(trimmed) > 32) return '昵称不能超过 32 个字符'
  return null
}

export function validateBio(value: string): string | null {
  return characterCount(value) > 200 ? '个人简介不能超过 200 个字符' : null
}

export function validateAvatarUrl(value: string): string | null {
  const trimmed = value.trim()
  if (trimmed === '') return null
  if (trimmed.length > 512) return '头像地址不能超过 512 个字符'

  try {
    const url = new URL(trimmed)
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || !url.hostname) {
      return '头像地址需为 http 或 https 链接'
    }
  } catch {
    return '头像地址需为 http 或 https 链接'
  }
  return null
}

export function validateCoverUrl(value: string): string | null {
  const trimmed = value.trim()
  if (trimmed === '') return null
  if (trimmed.length > 512) return '封面地址需为 http 或 https 链接'

  try {
    const url = new URL(trimmed)
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || !url.hostname) {
      return '封面地址需为 http 或 https 链接'
    }
  } catch {
    return '封面地址需为 http 或 https 链接'
  }
  return null
}
