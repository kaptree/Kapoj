import { describe, expect, it } from 'vitest'

import type { TaskStatus } from '@/api/types'
import {
  achievementConditionLabel,
  badgeImageSrc,
  badgeInitial,
  coinReasonLabel,
  formatCoins,
  safeTitleColor,
  taskStatusLabel,
  taskStatusTagType,
  titleUnlockLabel,
} from './gamification'

describe('gamification utils', () => {
  it('maps task statuses and tag types', () => {
    const statuses: {
      status: TaskStatus
      label: string
      tag: ReturnType<typeof taskStatusTagType>
    }[] = [
      { status: 'upcoming', label: '未开始', tag: 'info' },
      { status: 'active', label: '可领取', tag: 'success' },
      { status: 'claimed', label: '进行中', tag: 'warning' },
      { status: 'completed', label: '已完成', tag: 'success' },
      { status: 'expired', label: '已过期', tag: 'danger' },
      { status: 'ended', label: '已结束', tag: 'info' },
      { status: 'inactive', label: '已下线', tag: 'info' },
    ]

    for (const { status, label, tag } of statuses) {
      expect(taskStatusLabel(status)).toBe(label)
      expect(taskStatusTagType(status)).toBe(tag)
    }
  })

  it('maps achievement and title condition labels', () => {
    expect(achievementConditionLabel('ac_count')).toBe('AC 题目数')
    expect(achievementConditionLabel('contest_count')).toBe('参赛数')
    expect(achievementConditionLabel('login_streak')).toBe('连续登录')
    expect(achievementConditionLabel('created_contest')).toBe('创建比赛')
    expect(achievementConditionLabel('all_of')).toBe('组合条件')

    expect(titleUnlockLabel('rating')).toBe('Rating')
    expect(titleUnlockLabel('ac_count')).toBe('AC 题目数')
    expect(titleUnlockLabel('achievement')).toBe('成就')
  })

  it('rejects unsafe title colors', () => {
    expect(safeTitleColor('#7C3AED')).toBe('#7C3AED')
    expect(safeTitleColor('#7c3aed')).toBe('#7c3aed')

    for (const invalid of [
      'red',
      '#FFF',
      '#12345',
      '#1234567',
      ' #123456',
      '#123456 ',
      '#GGGGGG',
      '',
    ]) {
      expect(safeTitleColor(invalid)).toBe('#303133')
    }
  })

  it('formats coin reasons', () => {
    expect(coinReasonLabel('achievement')).toBe('成就奖励')
    expect(coinReasonLabel('task')).toBe('任务奖励')
    expect(coinReasonLabel('badge_purchase')).toBe('购买徽章')
    expect(coinReasonLabel('title_purchase')).toBe('购买称号')
    expect(coinReasonLabel('admin_adjust')).toBe('管理员调整')
    expect(coinReasonLabel('unknown_reason')).toBe('unknown_reason')
  })

  it('formats coins and badge fallbacks', () => {
    expect(formatCoins(0)).toBe('0')
    expect(formatCoins(1234567)).toBe('1,234,567')
    expect(formatCoins(-1200)).toBe('-1,200')
    expect(formatCoins(Number.NaN)).toBe('0')

    expect(badgeInitial('早鸟')).toBe('早')
    expect(badgeInitial('  Explorer')).toBe('E')
    expect(badgeInitial('')).toBe('?')

    expect(badgeImageSrc('')).toBeUndefined()
    expect(badgeImageSrc('   ')).toBeUndefined()
    expect(badgeImageSrc('/api/badges/1/image')).toBe('/api/badges/1/image')
  })
})
