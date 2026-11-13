import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { EvaluationCloudItem } from '@/api/types'
import { buildProfileWordCloudLayout } from '@/utils/profileWordCloudLayout'

import ProfileWordCloud from './ProfileWordCloud.vue'

vi.mock('@/utils/profileWordCloudLayout', () => ({
  buildProfileWordCloudLayout: vi.fn(),
}))

function cloudItem(text: string, count = 1): EvaluationCloudItem {
  return { text, count }
}

function placement(text: string, count: number, index: number) {
  return {
    text,
    count,
    x: 100 + index * 80,
    y: 110,
    width: 90,
    height: 24,
    fontSize: 16,
    rotation: index % 2 === 0 ? 0 : 3,
    colorIndex: index % 5,
  }
}

function mockLayout(overflow = 0) {
  vi.mocked(buildProfileWordCloudLayout).mockImplementation((items) => ({
    placements: items.map((item, index) => placement(item.text, item.count, index)),
    overflow,
  }))
}

function mountCloud(
  props: Partial<{
    cloud: EvaluationCloudItem[]
    total: number
    loading: boolean
    error: string
  }> = {},
) {
  return mount(ProfileWordCloud, {
    props: {
      cloud: [cloudItem('讲题清楚', 3), cloudItem('代码干净', 1)],
      total: 4,
      ...props,
    },
  })
}

beforeEach(() => {
  mockLayout()
})

describe('ProfileWordCloud', () => {
  it('renders words, total and the click hint', () => {
    const wrapper = mountCloud()

    expect(wrapper.get('[data-test="profile-word-cloud"]').attributes('aria-label')).toBe(
      '评价词云，共 4 条评价',
    )
    expect(wrapper.findAll('.profile-word-cloud__word')).toHaveLength(2)
    expect(wrapper.text()).toContain('讲题清楚')
    expect(wrapper.text()).toContain('代码干净')
    expect(wrapper.text()).toContain('4 条评价')
    expect(wrapper.text()).toContain('点击词云，写一句评价')

    wrapper.unmount()
  })

  it('emits open when the panel is clicked', async () => {
    const wrapper = mountCloud()

    await wrapper.get('[data-test="profile-word-cloud"]').trigger('click')

    expect(wrapper.emitted('open')).toHaveLength(1)
    wrapper.unmount()
  })

  it('emits open from the write-evaluation button', async () => {
    const wrapper = mountCloud()

    await wrapper.get('.profile-word-cloud__cta').trigger('click')

    expect(wrapper.emitted('open')).toHaveLength(1)
    wrapper.unmount()
  })

  it('keeps the empty state clickable', async () => {
    const wrapper = mountCloud({ cloud: [], total: 0 })

    expect(wrapper.text()).toContain('还没有评价')
    expect(wrapper.text()).toContain('点击写下第一条评价')

    await wrapper.get('[data-test="profile-word-cloud"]').trigger('click')
    expect(wrapper.emitted('open')).toHaveLength(1)
    wrapper.unmount()
  })

  it('shows loading without words', () => {
    const wrapper = mountCloud({ loading: true })

    expect(wrapper.text()).toContain('正在加载评价')
    expect(wrapper.findAll('.profile-word-cloud__word')).toHaveLength(0)
    wrapper.unmount()
  })

  it('shows an error with a retry button that does not open the dialog', async () => {
    const wrapper = mountCloud({ error: '评价加载失败' })

    expect(wrapper.text()).toContain('评价加载失败')

    await wrapper.get('.profile-word-cloud__retry').trigger('click')

    expect(wrapper.emitted('retry')).toHaveLength(1)
    expect(wrapper.emitted('open')).toBeUndefined()
    wrapper.unmount()
  })

  it('keeps the full long text in the title and marks the word for truncation', () => {
    const longText = '这是一条非常长的评价内容用来验证词云省略'
    mockLayout()
    const wrapper = mountCloud({ cloud: [cloudItem(longText, 1)], total: 1 })

    const word = wrapper.get('.profile-word-cloud__word')
    expect(word.attributes('title')).toBe(longText)
    expect(word.classes()).toContain('profile-word-cloud__word--truncate')
    wrapper.unmount()
  })

  it('shows the overflow count', () => {
    mockLayout(3)
    const wrapper = mountCloud()

    expect(wrapper.get('.profile-word-cloud__overflow').text()).toBe('+3')
    wrapper.unmount()
  })
})
