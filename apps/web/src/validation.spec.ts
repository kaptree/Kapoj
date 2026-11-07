import { describe, expect, it } from 'vitest'

import {
  byteLength,
  validateAvatarUrl,
  validateBio,
  validateCoverUrl,
  validateEmail,
  validateLoginAccount,
  validateLoginPassword,
  validateNickname,
  validatePassword,
  validateProfileNickname,
  validateUsername,
} from './validation'

describe('validateUsername', () => {
  it('accepts 3–32 letters, digits and underscores', () => {
    for (const value of ['abc', 'Alice_01', 'a'.repeat(32)]) {
      expect(validateUsername(value)).toBeNull()
    }
  })

  it('rejects empty and malformed usernames', () => {
    const message = '用户名需为 3–32 位字母、数字或下划线'

    expect(validateUsername('')).toBe('请输入用户名')
    expect(validateUsername('ab')).toBe(message)
    expect(validateUsername('a'.repeat(33))).toBe(message)
    expect(validateUsername('alice-01')).toBe(message)
    expect(validateUsername(' alice')).toBe(message)
    expect(validateUsername('爱丽丝')).toBe(message)
  })
})

describe('validateEmail', () => {
  it('accepts common addresses', () => {
    for (const value of ['alice@example.com', 'a.b+tag@example.co', 'user_name@sub.example.com']) {
      expect(validateEmail(value)).toBeNull()
    }
  })

  it('rejects empty, malformed and overlong addresses', () => {
    expect(validateEmail('')).toBe('请输入邮箱')
    for (const value of ['alice', 'alice@', 'alice@example', 'a b@example.com']) {
      expect(validateEmail(value)).toBe('邮箱格式不正确')
    }
    expect(validateEmail(`${'a'.repeat(250)}@example.com`)).toBe('邮箱格式不正确')
  })
})

describe('validatePassword', () => {
  it('measures the UTF-8 byte length', () => {
    expect(byteLength('abc')).toBe(3)
    expect(byteLength('密码')).toBe(6)
  })

  it('accepts 8–72 bytes and rejects everything else', () => {
    expect(validatePassword('12345678')).toBeNull()
    expect(validatePassword('a'.repeat(72))).toBeNull()
    expect(validatePassword('密码密码')).toBeNull()

    expect(validatePassword('')).toBe('请输入密码')
    expect(validatePassword('1234567')).toBe('密码长度需为 8–72 个字节')
    expect(validatePassword('a'.repeat(73))).toBe('密码长度需为 8–72 个字节')
    expect(validatePassword('密'.repeat(25))).toBe('密码长度需为 8–72 个字节')
  })
})

describe('login validation', () => {
  it('requires an account and password without applying registration rules', () => {
    expect(validateLoginAccount('')).toBe('请输入用户名或邮箱')
    expect(validateLoginAccount('   ')).toBe('请输入用户名或邮箱')
    expect(validateLoginAccount('alice')).toBeNull()

    expect(validateLoginPassword('')).toBe('请输入密码')
    expect(validateLoginPassword('short')).toBeNull()
  })
})

describe('validateNickname', () => {
  it('is optional and limits the value to 32 characters', () => {
    expect(validateNickname('')).toBeNull()
    expect(validateNickname('Alice')).toBeNull()
    expect(validateNickname('😀'.repeat(32))).toBeNull()
    expect(validateNickname('😀'.repeat(33))).toBe('昵称不能超过 32 个字符')
  })
})

describe('validateProfileNickname', () => {
  it('requires a non-blank nickname of at most 32 characters', () => {
    expect(validateProfileNickname('')).toBe('请输入昵称')
    expect(validateProfileNickname('   ')).toBe('请输入昵称')
    expect(validateProfileNickname(' Alice ')).toBeNull()
    expect(validateProfileNickname('a'.repeat(32))).toBeNull()
    expect(validateProfileNickname('a'.repeat(33))).toBe('昵称不能超过 32 个字符')
  })
})

describe('validateBio', () => {
  it('allows at most 200 characters', () => {
    expect(validateBio('')).toBeNull()
    expect(validateBio('a'.repeat(200))).toBeNull()
    expect(validateBio('a'.repeat(201))).toBe('个人简介不能超过 200 个字符')
  })
})

describe('validateAvatarUrl', () => {
  it('allows empty values and http(s) URLs', () => {
    expect(validateAvatarUrl('')).toBeNull()
    expect(validateAvatarUrl('   ')).toBeNull()
    expect(validateAvatarUrl('https://example.com/avatar.png')).toBeNull()
    expect(validateAvatarUrl('http://localhost:8080/avatar.png')).toBeNull()
  })

  it('rejects other schemes, malformed values and overlong URLs', () => {
    const message = '头像地址需为 http 或 https 链接'

    expect(validateAvatarUrl('ftp://example.com/avatar.png')).toBe(message)
    expect(validateAvatarUrl('example.com/avatar.png')).toBe(message)
    expect(validateAvatarUrl('https://')).toBe(message)
    expect(validateAvatarUrl(`https://example.com/${'a'.repeat(500)}`)).toBe(
      '头像地址不能超过 512 个字符',
    )
  })
})

describe('validateCoverUrl', () => {
  const message = '封面地址需为 http 或 https 链接'

  it('allows empty values and http(s) URLs', () => {
    expect(validateCoverUrl('')).toBeNull()
    expect(validateCoverUrl('   ')).toBeNull()
    expect(validateCoverUrl('https://example.com/a.png')).toBeNull()
    expect(validateCoverUrl('http://localhost:8080/a.png')).toBeNull()
  })

  it('rejects other schemes and malformed values', () => {
    expect(validateCoverUrl('ftp://example.com/a.png')).toBe(message)
    expect(validateCoverUrl('javascript:alert(1)')).toBe(message)
    expect(validateCoverUrl('not-a-url')).toBe(message)
    expect(validateCoverUrl('https://')).toBe(message)
  })

  it('rejects URLs longer than 512 characters', () => {
    expect(validateCoverUrl(`https://example.com/${'a'.repeat(500)}`)).toBe(message)
    expect(validateCoverUrl('a'.repeat(513))).toBe(message)
  })
})
