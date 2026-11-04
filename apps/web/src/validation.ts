import type { FormItemRule } from 'element-plus'

const byteLength = (s: string) => new TextEncoder().encode(s).length

// These mirror the server rules in apps/server/internal/user/validate.go.
export const usernameRules: FormItemRule[] = [
  { required: true, message: '请输入用户名', trigger: 'blur' },
  {
    pattern: /^[a-zA-Z0-9_]{3,32}$/,
    message: '用户名需为 3–32 位字母、数字或下划线',
    trigger: 'blur',
  },
]

export const emailRules: FormItemRule[] = [
  { required: true, message: '请输入邮箱', trigger: 'blur' },
  { type: 'email', message: '邮箱格式不正确', trigger: 'blur' },
]

export const passwordRules: FormItemRule[] = [
  { required: true, message: '请输入密码', trigger: 'blur' },
  {
    validator: (_rule, value: string, callback) => {
      const n = byteLength(value ?? '')
      if (n < 8 || n > 72) callback(new Error('密码长度需为 8–72 个字节'))
      else callback()
    },
    trigger: 'blur',
  },
]

export const nicknameRules: FormItemRule[] = [
  { max: 32, message: '昵称不能超过 32 个字符', trigger: 'blur' },
]
