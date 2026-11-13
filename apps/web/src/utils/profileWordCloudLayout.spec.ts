import { describe, expect, it } from 'vitest'

import {
  buildProfileWordCloudLayout,
  createProfileWordCloudRandom,
  estimateProfileWordWidth,
  profileWordCloudColorIndex,
  type ProfileWordCloudItem,
  type ProfileWordCloudPlacement,
} from './profileWordCloudLayout'

function item(text: string, count = 1): ProfileWordCloudItem {
  return { text, count }
}

function boxesOverlap(a: ProfileWordCloudPlacement, b: ProfileWordCloudPlacement): boolean {
  const overlapX = Math.abs(a.x - b.x) < (a.width + b.width) / 2
  const overlapY = Math.abs(a.y - b.y) < (a.height + b.height) / 2
  return overlapX && overlapY
}

describe('profile word cloud layout', () => {
  it('places 24 short items without overlap and inside the canvas', () => {
    const items = Array.from({ length: 24 }, (_, index) =>
      item(`评价${index + 1}`, (index % 5) + 1),
    )

    const layout = buildProfileWordCloudLayout(items, 640, 220)

    expect(layout.placements).toHaveLength(24)
    expect(layout.overflow).toBe(0)

    for (const placement of layout.placements) {
      expect(placement.x - placement.width / 2).toBeGreaterThanOrEqual(0)
      expect(placement.x + placement.width / 2).toBeLessThanOrEqual(640)
      expect(placement.y - placement.height / 2).toBeGreaterThanOrEqual(0)
      expect(placement.y + placement.height / 2).toBeLessThanOrEqual(220)
    }

    for (let i = 0; i < layout.placements.length; i += 1) {
      for (let j = i + 1; j < layout.placements.length; j += 1) {
        expect(boxesOverlap(layout.placements[i], layout.placements[j])).toBe(false)
      }
    }
  })

  it('is deterministic for the same input', () => {
    const items = [item('讲题清楚', 4), item('代码干净', 2), item('大佬', 1)]

    const first = buildProfileWordCloudLayout(items, 480, 200)
    const second = buildProfileWordCloudLayout(items, 480, 200)

    expect(second.placements).toEqual(first.placements)
    expect(second.overflow).toBe(first.overflow)
  })

  it('caps long text width and shrinks its font size', () => {
    const longText = '这是一条非常长的评价内容用来验证词云不会因为文本过长而撑破容器边界'
    const longLayout = buildProfileWordCloudLayout([item(longText, 3)], 640, 220)
    const shortLayout = buildProfileWordCloudLayout([item('短评价', 3)], 640, 220)

    expect(longLayout.placements).toHaveLength(1)
    expect(longLayout.placements[0].width).toBeLessThanOrEqual(640 * 0.45)
    expect(longLayout.placements[0].fontSize).toBeLessThanOrEqual(
      shortLayout.placements[0].fontSize,
    )
  })

  it('reports overflow in a small canvas without placing items outside it', () => {
    const items = Array.from({ length: 24 }, (_, index) =>
      item(`很长的评价文本${index + 1}`, (index % 4) + 1),
    )

    const layout = buildProfileWordCloudLayout(items, 200, 120)

    expect(layout.overflow).toBeGreaterThan(0)
    expect(layout.placements.length).toBeLessThan(items.length)
    for (const placement of layout.placements) {
      expect(placement.x - placement.width / 2).toBeGreaterThanOrEqual(0)
      expect(placement.x + placement.width / 2).toBeLessThanOrEqual(200)
      expect(placement.y - placement.height / 2).toBeGreaterThanOrEqual(0)
      expect(placement.y + placement.height / 2).toBeLessThanOrEqual(120)
    }
  })

  it('keeps rotation within -8 and 8 degrees', () => {
    const layout = buildProfileWordCloudLayout(
      Array.from({ length: 12 }, (_, index) => item(`评价${index}`, (index % 3) + 1)),
      640,
      220,
    )

    for (const placement of layout.placements) {
      expect(placement.rotation).toBeGreaterThanOrEqual(-8)
      expect(placement.rotation).toBeLessThanOrEqual(8)
    }
  })

  it('counts items beyond maxWords as overflow', () => {
    const items = Array.from({ length: 24 }, (_, index) => item(`评价${index + 1}`, 1))
    const layout = buildProfileWordCloudLayout(items, 640, 220, { maxWords: 10 })

    expect(layout.placements.length).toBeLessThanOrEqual(10)
    expect(layout.placements.length + layout.overflow).toBe(items.length)
  })

  it('returns an empty layout for no items', () => {
    expect(buildProfileWordCloudLayout([], 640, 220)).toEqual({ placements: [], overflow: 0 })
  })

  it('estimates wider boxes for CJK text and longer strings', () => {
    expect(estimateProfileWordWidth('评价', 20)).toBeGreaterThan(estimateProfileWordWidth('ab', 20))
    expect(estimateProfileWordWidth('评价评价', 20)).toBeGreaterThan(
      estimateProfileWordWidth('评价', 20),
    )
  })

  it('keeps color indexes stable and within the palette', () => {
    const first = profileWordCloudColorIndex('讲题清楚')
    const second = profileWordCloudColorIndex('讲题清楚')

    expect(first).toBe(second)
    expect(first).toBeGreaterThanOrEqual(0)
    expect(first).toBeLessThanOrEqual(4)
  })

  it('generates a stable pseudo-random sequence per seed', () => {
    const first = createProfileWordCloudRandom(42)
    const second = createProfileWordCloudRandom(42)
    const third = createProfileWordCloudRandom(43)

    expect([first(), first(), first()]).toEqual([second(), second(), second()])
    expect(first()).not.toBe(third())
  })
})
