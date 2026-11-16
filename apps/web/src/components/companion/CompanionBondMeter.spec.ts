import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CompanionBondMeter from './CompanionBondMeter.vue'

const base = {
  bond: 120,
  level: 3,
  levelName: '搭档',
  nextThreshold: 170,
  dailyGain: 4,
  dailyCap: 15,
}

function mountMeter(overrides: Partial<typeof base> = {}) {
  return mount(CompanionBondMeter, { props: { ...base, ...overrides } })
}

describe('CompanionBondMeter', () => {
  it('renders level, name and bond progress', () => {
    const wrapper = mountMeter()

    expect(wrapper.text()).toContain('Lv.3 搭档 120/170')

    const bar = wrapper.get('[role="progressbar"]')
    expect(bar.attributes('aria-valuenow')).toBe('120')
    expect(bar.attributes('aria-valuemin')).toBe('0')
    expect(bar.attributes('aria-valuemax')).toBe('170')
    expect(bar.attributes('aria-label')).toContain('搭档')
  })

  it('fills the track proportionally to the next threshold', () => {
    const fill = mountMeter().get('.companion-bond__fill')
    expect(fill.attributes('style')).toContain('width: 70.588')
  })

  it('never overflows a full track', () => {
    const fill = mountMeter({ bond: 200 }).get('.companion-bond__fill')
    expect(fill.attributes('style')).toContain('width: 100%')
  })

  it('tolerates a zero threshold and empty day', () => {
    const wrapper = mountMeter({
      bond: 0,
      level: 0,
      levelName: '陌生',
      nextThreshold: 0,
      dailyGain: 0,
      dailyCap: 0,
    })

    expect(wrapper.get('.companion-bond__fill').attributes('style')).toContain('width: 0%')
    expect(wrapper.text()).toContain('今日 0/0')
  })

  it('shows the daily natural gain against the cap', () => {
    expect(mountMeter().text()).toContain('今日 4/15')
  })
})
