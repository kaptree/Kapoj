import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'

import type { ActivityDay, UserActivity } from '@/api/types'
import ContributionGrid from './ContributionGrid.vue'
import contributionSource from './ContributionGrid.vue?raw'

const START_DATE = '2025-11-13'
const END_DATE = '2026-11-12'

const COUNTS: Record<string, number> = {
  '2026-10-31': 2,
  '2026-11-01': 4,
  '2026-11-02': 8,
  '2026-11-03': 12,
}

function toDateKey(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

/**
 * The API guarantees 365 days, but only non-zero days are included here so the
 * grid's zero-filling behaviour is exercised.
 */
function activity(overrides: Partial<UserActivity> = {}): UserActivity {
  const days: ActivityDay[] = []
  const cursor = new Date(`${START_DATE}T00:00:00`)
  for (let index = 0; index < 365; index += 1) {
    const date = toDateKey(cursor)
    const count = COUNTS[date]
    if (count !== undefined) days.push({ date, count })
    cursor.setDate(cursor.getDate() + 1)
  }

  return {
    submissionCount: 120,
    acceptedProblemCount: 80,
    contributionTotal: 26,
    startDate: START_DATE,
    endDate: END_DATE,
    currentStreak: 4,
    longestStreak: 18,
    days,
    ...overrides,
  }
}

function mountGrid(value = activity()) {
  return mount(ContributionGrid, { props: { activity: value } })
}

function dayCell(wrapper: ReturnType<typeof mountGrid>, date: string) {
  return wrapper.get(`.contribution-cells .contribution-cell[aria-label^="${date}："]`)
}

function hexLuminance(hex: string): number {
  const value = hex.replace('#', '')
  const channels = [0, 2, 4].map((start) => parseInt(value.slice(start, start + 2), 16) / 255)
  const [r, g, b] = channels.map((channel) =>
    channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4,
  )
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function darkPalette(): number[] {
  const start = contributionSource.indexOf('.contribution-grid {')
  const end = contributionSource.indexOf(':root:not(.app-dark)')
  const block = contributionSource.slice(start, end)
  return [1, 2, 3, 4].map((level) => {
    const match = block.match(new RegExp(`--contrib-${level}:\\s*(#[0-9A-Fa-f]{6})`))
    if (!match) throw new Error(`missing --contrib-${level}`)
    return hexLuminance(match[1])
  })
}

describe('ContributionGrid', () => {
  it('renders one 365-day grid, padding missing days with level 0', () => {
    const wrapper = mountGrid()

    const cells = wrapper.findAll(
      '.contribution-cells .contribution-cell:not(.contribution-cell--blank)',
    )
    expect(cells).toHaveLength(365)
    expect(wrapper.findAll('.contribution-cells .is-level-0').length).toBeGreaterThan(0)

    expect(dayCell(wrapper, '2026-10-31').classes()).toContain('is-level-1')
    expect(dayCell(wrapper, '2026-11-01').classes()).toContain('is-level-2')
    expect(dayCell(wrapper, '2026-11-02').classes()).toContain('is-level-3')
    expect(dayCell(wrapper, '2026-11-03').classes()).toContain('is-level-4')
    expect(dayCell(wrapper, '2026-10-30').classes()).toContain('is-level-0')
  })

  it('labels every day cell with its date and count', () => {
    const wrapper = mountGrid()

    const cell = dayCell(wrapper, '2026-11-03')
    expect(cell.attributes('title')).toBe('2026-11-03：12 次代码提交')
    expect(cell.attributes('aria-label')).toBe('2026-11-03：12 次代码提交')

    const empty = dayCell(wrapper, '2026-10-30')
    expect(empty.attributes('title')).toBe('2026-10-30：0 次代码提交')
  })

  it('renders month and weekday labels', () => {
    const wrapper = mountGrid()

    const months = wrapper.findAll('.contribution-month').map((label) => label.text())
    expect(months.length).toBeGreaterThanOrEqual(12)
    expect(months).toContain('11月')
    expect(months).toContain('1月')

    const weekdays = wrapper.findAll('.contribution-weekday').map((label) => label.text())
    expect(weekdays.join('')).toBe('一三五')
  })

  it('uses a darker color for more submissions in the dark theme', () => {
    const palette = darkPalette()

    for (let index = 1; index < palette.length; index += 1) {
      expect(palette[index]).toBeLessThan(palette[index - 1])
    }
  })

  it('renders the summary, legend and accessible title', () => {
    const wrapper = mountGrid()

    expect(wrapper.get('.contribution-grid__title').text()).toBe('代码提交贡献图')
    expect(wrapper.get('.contribution-grid__summary').text()).toBe(
      '过去一年 26 次代码提交 · 当前连续 4 天 · 最长连续 18 天',
    )

    const legend = wrapper.get('.contribution-legend')
    expect(legend.text()).toContain('少')
    expect(legend.text()).toContain('多')
    expect(wrapper.findAll('.contribution-legend__swatch')).toHaveLength(5)
  })

  it('shows annual and streak stats beside the title', () => {
    const wrapper = mountGrid()

    const stats = wrapper.findAll('.contribution-stat')
    expect(stats).toHaveLength(3)
    expect(stats[0]!.text()).toContain('年度提交')
    expect(stats[0]!.text()).toContain('26')
    expect(stats[1]!.text()).toContain('当前连续')
    expect(stats[1]!.text()).toContain('4 天')
    expect(stats[2]!.text()).toContain('最长连续')
    expect(stats[2]!.text()).toContain('18 天')
  })

  it('shows the hovered date and submission count in the footer', async () => {
    const wrapper = mountGrid()
    const cell = dayCell(wrapper, '2026-11-03')

    await cell.trigger('mouseenter')
    expect(wrapper.get('[data-test="contribution-hover"]').text()).toContain(
      '2026-11-03 · 12 次代码提交',
    )

    await cell.trigger('mouseleave')
    expect(wrapper.get('[data-test="contribution-hover"]').text()).toContain('每格代表一天')
  })

  it('uses one roving tab stop and lets keyboard users inspect days', async () => {
    const wrapper = mountGrid()
    const active = dayCell(wrapper, '2026-11-03')
    const empty = dayCell(wrapper, '2026-10-30')

    expect(active.attributes('tabindex')).toBe('0')
    expect(empty.attributes('tabindex')).toBe('-1')
    expect(wrapper.findAll('.contribution-cell[tabindex="0"]')).toHaveLength(1)

    await active.trigger('focus')
    expect(wrapper.get('[data-test="contribution-hover"]').text()).toContain(
      '2026-11-03 · 12 次代码提交',
    )

    await active.trigger('keydown', { key: 'ArrowLeft' })
    await nextTick()
    expect(dayCell(wrapper, '2026-11-02').attributes('tabindex')).toBe('0')
    expect(active.attributes('tabindex')).toBe('-1')
  })

  it('caps the grid at 365 days when the window is wider', () => {
    const value = activity({ startDate: '2024-01-01', endDate: '2026-11-12' })
    const wrapper = mountGrid(value)

    expect(
      wrapper.findAll('.contribution-cells .contribution-cell:not(.contribution-cell--blank)'),
    ).toHaveLength(365)
  })
})
