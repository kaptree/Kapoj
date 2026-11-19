import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { requestCompanionDialogue } from '@/api/companion'
import { COMPANION_LINES } from '@/content/companion/lines.zh-CN'
import { useCompanionStore } from '@/stores/companion'
import { useCompanionDialogue } from './useCompanionDialogue'

vi.mock('@/api/companion', () => ({
  requestCompanionDialogue: vi.fn(),
}))

let dialogue!: ReturnType<typeof useCompanionDialogue>

function mountDialogue() {
  const Comp = defineComponent({
    setup() {
      dialogue = useCompanionDialogue()
      return () => h('div')
    },
  })
  return mount(Comp)
}

beforeEach(() => {
  setActivePinia(createPinia())
  const store = useCompanionStore()
  store.aiReady = true
  store.aiDialogueEnabled = true
  vi.mocked(requestCompanionDialogue).mockReset()
})

describe('useCompanionDialogue', () => {
  it('returns an AI line and remembers it', async () => {
    vi.mocked(requestCompanionDialogue).mockResolvedValue({
      source: 'ai',
      line: '……就一下。',
      remainingToday: 29,
    })
    const wrapper = mountDialogue()

    const result = await dialogue.request({ trigger: 'pet', expression: 'shy' })
    expect(result).toEqual({ line: '……就一下。', source: 'ai' })
    expect(dialogue.recentLines.value).toEqual(['……就一下。'])
    wrapper.unmount()
  })

  it('uses the server lineKey for a static fallback', async () => {
    vi.mocked(requestCompanionDialogue).mockResolvedValue({
      source: 'default',
      lineKey: 'pet.shy',
      remainingToday: 0,
    })
    const wrapper = mountDialogue()

    const result = await dialogue.request({ trigger: 'pet', expression: 'shy' })
    expect(result.source).toBe('default')
    expect(COMPANION_LINES['pet.shy']).toContain(result.line)
    wrapper.unmount()
  })

  it('falls back to a static pet line on a network error', async () => {
    vi.mocked(requestCompanionDialogue).mockRejectedValue(new Error('offline'))
    const wrapper = mountDialogue()

    const result = await dialogue.request({ trigger: 'pet', expression: 'pout' })
    expect(result.source).toBe('default')
    expect(COMPANION_LINES['pet.pout']).toContain(result.line)
    wrapper.unmount()
  })

  it('does not call the API when AI is not ready', async () => {
    useCompanionStore().aiReady = false
    const wrapper = mountDialogue()

    const result = await dialogue.request({ trigger: 'pet', expression: 'happy' })
    expect(result.source).toBe('default')
    expect(COMPANION_LINES['pet.happy']).toContain(result.line)
    expect(requestCompanionDialogue).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('aborts the request and does not remember lines after unmount', async () => {
    let signal: AbortSignal | undefined
    vi.mocked(requestCompanionDialogue).mockImplementation((_input, requestSignal) => {
      signal = requestSignal
      return new Promise((_resolve, reject) => {
        requestSignal?.addEventListener('abort', () =>
          reject(new DOMException('Aborted', 'AbortError')),
        )
      })
    })
    const wrapper = mountDialogue()
    const pending = dialogue.request({ trigger: 'pet', expression: 'shy' })

    wrapper.unmount()
    expect(signal?.aborted).toBe(true)
    await expect(pending).resolves.toMatchObject({ source: 'default' })
    expect(dialogue.recentLines.value).toEqual([])
  })
})
