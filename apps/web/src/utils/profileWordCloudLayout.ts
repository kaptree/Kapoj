export interface ProfileWordCloudItem {
  text: string
  count: number
}

export interface ProfileWordCloudPlacement {
  text: string
  count: number
  x: number
  y: number
  width: number
  height: number
  fontSize: number
  rotation: number
  colorIndex: number
}

export interface ProfileWordCloudLayout {
  placements: ProfileWordCloudPlacement[]
  overflow: number
}

export interface ProfileWordCloudOptions {
  maxWords?: number
  gap?: number
  maxWidthRatio?: number
  minFontSize?: number
  maxFontSize?: number
}

const DEFAULT_OPTIONS: Required<ProfileWordCloudOptions> = {
  maxWords: 24,
  gap: 5,
  maxWidthRatio: 0.45,
  minFontSize: 12,
  maxFontSize: 27,
}

const CJK_RE = /[\u2e80-\u2eff\u3040-\u30ff\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uac00-\ud7af]/

const COLOR_COUNT = 5

/** Approximate rendered width of a word at a given font size. */
export function estimateProfileWordWidth(text: string, fontSize: number): number {
  let units = 0
  for (const char of Array.from(text)) {
    if (CJK_RE.test(char)) {
      units += 1
    } else if (char === ' ') {
      units += 0.32
    } else {
      units += 0.58
    }
  }
  return Math.max(fontSize, units * fontSize)
}

/** Stable colour bucket for a word. */
export function profileWordCloudColorIndex(text: string): number {
  let hash = 0
  for (const char of text) {
    hash = (hash * 31 + char.codePointAt(0)!) >>> 0
  }
  return hash % COLOR_COUNT
}

/** Deterministic mulberry32 sequence; same seed gives the same layout jitter. */
export function createProfileWordCloudRandom(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let value = state
    value = Math.imul(value ^ (value >>> 15), value | 1)
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61)
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296
  }
}

interface SizedWord {
  text: string
  count: number
  fontSize: number
  width: number
  height: number
  rotation: number
  colorIndex: number
}

interface Box {
  x: number
  y: number
  width: number
  height: number
}

function textSeed(text: string): number {
  let hash = 2166136261
  for (const char of text) {
    hash ^= char.codePointAt(0)!
    hash = Math.imul(hash, 16777619)
  }
  return hash >>> 0
}

function sizeWord(
  item: ProfileWordCloudItem,
  index: number,
  width: number,
  options: Required<ProfileWordCloudOptions>,
): SizedWord {
  const countBonus = Math.min(Math.max(item.count, 1) - 1, 4) * 2.5
  let fontSize = Math.min(options.maxFontSize, 15 + countBonus)
  const maxWidth = Math.max(options.minFontSize, width * options.maxWidthRatio)

  const rawWidth = estimateProfileWordWidth(item.text, fontSize)
  if (rawWidth > maxWidth) {
    fontSize = Math.max(options.minFontSize, (fontSize * maxWidth) / rawWidth)
  }

  const random = createProfileWordCloudRandom(textSeed(item.text))
  const rotation = index % 3 === 0 ? 0 : Number((random() * 16 - 8).toFixed(1))
  const cappedWidth = Math.min(estimateProfileWordWidth(item.text, fontSize), maxWidth)

  return {
    text: item.text,
    count: item.count,
    fontSize,
    width: cappedWidth,
    height: fontSize * 1.35,
    rotation,
    colorIndex: profileWordCloudColorIndex(item.text),
  }
}

function collides(box: Box, placed: Box[], gap: number): boolean {
  for (const other of placed) {
    const overlapX = Math.abs(box.x - other.x) < (box.width + other.width) / 2 + gap
    const overlapY = Math.abs(box.y - other.y) < (box.height + other.height) / 2 + gap
    if (overlapX && overlapY) return true
  }
  return false
}

function placeWords(
  words: SizedWord[],
  width: number,
  height: number,
  gap: number,
): { placed: SizedWord[]; positions: Array<{ x: number; y: number }> } {
  const placed: SizedWord[] = []
  const positions: Array<{ x: number; y: number }> = []
  const boxes: Box[] = []
  const centerX = width / 2
  const centerY = height / 2
  const maxSteps = 2600

  for (const word of words) {
    if (word.width > width - gap * 2 || word.height > height - gap * 2) continue

    let found: { x: number; y: number } | null = null
    for (let step = 0; step < maxSteps; step += 1) {
      const angle = step * 0.35
      const radius = step * 2.2
      const x = Math.max(
        word.width / 2,
        Math.min(width - word.width / 2, centerX + Math.cos(angle) * radius),
      )
      const y = Math.max(
        word.height / 2,
        Math.min(height - word.height / 2, centerY + Math.sin(angle) * radius * 0.7),
      )
      const box: Box = { x, y, width: word.width, height: word.height }
      if (!collides(box, boxes, gap)) {
        found = { x, y }
        break
      }
    }

    if (!found) continue
    placed.push(word)
    positions.push(found)
    boxes.push({ x: found.x, y: found.y, width: word.width, height: word.height })
  }

  return { placed, positions }
}

/**
 * Deterministic irregular word cloud layout. Words are placed on an
 * Archimedean spiral from the centre, with axis-aligned collision boxes.
 * Rotation only affects presentation, never collision.
 */
export function buildProfileWordCloudLayout(
  items: ProfileWordCloudItem[],
  width: number,
  height: number,
  options: ProfileWordCloudOptions = {},
): ProfileWordCloudLayout {
  if (items.length === 0 || width <= 0 || height <= 0) {
    return { placements: [], overflow: items.length }
  }

  const config: Required<ProfileWordCloudOptions> = { ...DEFAULT_OPTIONS, ...options }
  const sorted = [...items].sort((a, b) => b.count - a.count || a.text.localeCompare(b.text))
  const considered = sorted.slice(0, config.maxWords)
  let overflow = items.length - considered.length

  const words = considered.map((item, index) => sizeWord(item, index, width, config))

  // Try progressively smaller type before giving up on an item.
  for (let attempt = 0; attempt < 4; attempt += 1) {
    const scale = 0.9 ** attempt
    const scaled = words.map((word) => {
      const fontSize = Math.max(config.minFontSize, word.fontSize * scale)
      const maxWidth = Math.max(config.minFontSize, width * config.maxWidthRatio)
      const scaledWidth = Math.min(estimateProfileWordWidth(word.text, fontSize), maxWidth)
      return {
        ...word,
        fontSize,
        width: scaledWidth,
        height: fontSize * 1.35,
      }
    })

    const result = placeWords(scaled, width, height, config.gap)
    if (result.placed.length === scaled.length || attempt === 3) {
      overflow += scaled.length - result.placed.length
      return {
        placements: result.placed.map((word, index) => ({
          text: word.text,
          count: word.count,
          x: result.positions[index].x,
          y: result.positions[index].y,
          width: word.width,
          height: word.height,
          fontSize: word.fontSize,
          rotation: word.rotation,
          colorIndex: word.colorIndex,
        })),
        overflow,
      }
    }
  }

  return { placements: [], overflow: items.length }
}
