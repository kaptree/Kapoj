import { onBeforeUnmount, ref } from 'vue'

import { requestCompanionDialogue } from '@/api/companion'
import {
  COMPANION_LINES,
  pickCompanionLine,
  type CompanionLineKey,
} from '@/content/companion/lines.zh-CN'
import type { CompanionQExpression } from '@/content/companion/types'
import { useCompanionStore } from '@/stores/companion'

export type DialogueTrigger = 'pet' | 'greet' | 'idle'
export type DialogueGreetingKey = 'greet.morning' | 'greet.afternoon' | 'greet.night'

export interface DialogueInput {
  trigger: DialogueTrigger
  expression?: CompanionQExpression
  greetingKey?: DialogueGreetingKey
}

export interface DialogueLine {
  line: string
  source: 'ai' | 'default'
}

const PET_FALLBACK_KEYS: Record<CompanionQExpression, CompanionLineKey> = {
  neutral: 'pet',
  happy: 'pet.happy',
  shy: 'pet.shy',
  pout: 'pet.pout',
  surprised: 'pet.surprised',
  sleepy: 'pet.sleepy',
  smug: 'pet.smug',
}

function fallbackKey(input: DialogueInput): CompanionLineKey {
  if (input.trigger === 'greet') return input.greetingKey ?? 'greeting'
  if (input.trigger === 'idle') return 'idle.lively'
  return input.expression ? (PET_FALLBACK_KEYS[input.expression] ?? 'pet') : 'pet'
}

function clampLine(line: string): string {
  const runes = Array.from(line.trim())
  return runes.length > 80 ? runes.slice(0, 80).join('') : runes.join('')
}

/**
 * Request one companion line. The composable owns the short-term line memory
 * (last three lines) and guarantees a static fallback on every error path.
 */
export function useCompanionDialogue() {
  const store = useCompanionStore()
  const pending = ref(false)
  const recentLines = ref<string[]>([])
  let active: AbortController | null = null
  let disposed = false
  let sequence = 0

  function remember(line: string): void {
    const clean = clampLine(line)
    if (!clean) return
    recentLines.value = [...recentLines.value, clean].slice(-3)
  }

  function localLine(input: DialogueInput): string {
    return pickCompanionLine(fallbackKey(input), String(Date.now()), recentLines.value.at(-1))
  }

  function cancel(): void {
    sequence += 1
    active?.abort()
    active = null
    pending.value = false
  }

  async function request(input: DialogueInput): Promise<DialogueLine> {
    const fallback = localLine(input)
    // Invalidate any older request before the AI-ready check. A later static
    // interaction must not let an older AI response land in the bubble.
    const current = ++sequence
    active?.abort()
    active = null
    if (!store.aiReady || !store.aiDialogueEnabled) {
      return { line: fallback, source: 'default' }
    }

    const controller = new AbortController()
    active = controller
    pending.value = true
    const timeout = window.setTimeout(() => controller.abort(), 4500)

    try {
      const result = await requestCompanionDialogue(
        {
          trigger: input.trigger,
          expression: input.expression,
          greetingKey: input.greetingKey,
          recentLines: recentLines.value,
        },
        controller.signal,
      )
      if (disposed || current !== sequence) {
        return { line: fallback, source: 'default' }
      }
      const key =
        result.lineKey && result.lineKey in COMPANION_LINES
          ? (result.lineKey as CompanionLineKey)
          : fallbackKey(input)
      const line =
        result.source === 'ai' && result.line
          ? result.line
          : pickCompanionLine(key, String(Date.now()), recentLines.value.at(-1))
      if (Number.isInteger(result.remainingToday) && result.remainingToday >= 0) {
        store.aiRemainingToday = result.remainingToday
      }
      remember(line)
      return { line, source: result.source === 'ai' ? 'ai' : 'default' }
    } catch {
      if (disposed || current !== sequence) {
        return { line: fallback, source: 'default' }
      }
      remember(fallback)
      return { line: fallback, source: 'default' }
    } finally {
      window.clearTimeout(timeout)
      if (current === sequence) pending.value = false
    }
  }

  onBeforeUnmount(() => {
    disposed = true
    active?.abort()
    active = null
  })

  return { pending, recentLines, request, cancel }
}
