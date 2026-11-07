import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'

import {
  getOfficialSolution,
  purchaseOfficialSolution,
  updateOfficialSolution,
} from '@/api/problems'
import type { OfficialSolutionView } from '@/api/types'
import ProblemOfficialSolutionView from './ProblemOfficialSolutionView.vue'

const route = reactive({ params: { id: '1' }, fullPath: '/problems/1/official-solution' })

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const auth = vi.hoisted(() => ({
  fetchMe: vi.fn<(id?: unknown) => Promise<unknown>>(() => Promise.resolve()),
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route }
})
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/components/MarkdownView.vue', () => ({
  default: { props: ['source', 'theme'], template: '<div>{{ source }}</div>' },
}))
vi.mock('@/components/MarkdownEditor.vue', () => ({
  default: {
    props: ['modelValue', 'theme', 'height'],
    emits: ['update:modelValue'],
    template:
      '<textarea class="markdown-editor-mock" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}))
vi.mock('@/api/problems', () => ({
  getOfficialSolution: vi.fn(),
  updateOfficialSolution: vi.fn(),
  purchaseOfficialSolution: vi.fn(),
}))

function view(overrides: Partial<OfficialSolutionView> = {}): OfficialSolutionView {
  return {
    priceCoins: 100,
    access: 'locked',
    canEdit: false,
    canPurchase: true,
    ...overrides,
  }
}

function mountView() {
  return mount(ProblemOfficialSolutionView, {
    global: {
      plugins: [PrimeVue, ToastService, createPinia()],
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
        InputNumber: {
          props: ['modelValue'],
          template:
            '<input type="number" class="input-number-mock" :value="modelValue" @input="$emit(\'update:modelValue\', Number($event.target.value))" />',
        },
      },
    },
  })
}

function button(wrapper: ReturnType<typeof mountView>, label: string) {
  return wrapper.findAll('button').find((b) => b.text().trim() === label)
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  auth.fetchMe.mockClear()
  vi.mocked(getOfficialSolution).mockReset()
  vi.mocked(updateOfficialSolution).mockReset()
  vi.mocked(purchaseOfficialSolution).mockReset()
})

describe('ProblemOfficialSolutionView', () => {
  it('renders the six access states with the right content visibility', async () => {
    const cases: {
      access: OfficialSolutionView['access']
      priceCoins: number
      expectText: string
      expectContent: boolean
    }[] = [
      { access: 'free', priceCoins: 0, expectText: '免费', expectContent: true },
      { access: 'accepted', priceCoins: 100, expectText: '已通过', expectContent: true },
      { access: 'purchased', priceCoins: 100, expectText: '已解锁', expectContent: true },
      { access: 'creator', priceCoins: 100, expectText: '编辑官方题解', expectContent: true },
      { access: 'locked', priceCoins: 100, expectText: '支付 100 金币查看', expectContent: false },
      { access: 'guest', priceCoins: 100, expectText: '登录', expectContent: false },
    ]

    for (const c of cases) {
      vi.mocked(getOfficialSolution).mockResolvedValue(
        view({
          access: c.access,
          priceCoins: c.priceCoins,
          canEdit: c.access === 'creator',
          canPurchase: c.access === 'locked',
          solutionMd: 'MARKDOWN_BODY',
          solutionCode: 'CODE_BODY',
        }),
      )
      const wrapper = mountView()
      await flushPromises()

      expect(wrapper.text()).toContain(c.expectText)
      if (c.expectContent) {
        expect(wrapper.text()).toContain('MARKDOWN_BODY')
        expect(wrapper.text()).toContain('CODE_BODY')
      } else {
        expect(wrapper.text()).not.toContain('MARKDOWN_BODY')
        expect(wrapper.text()).not.toContain('CODE_BODY')
      }
      wrapper.unmount()
    }
  })

  it('lets the creator edit md/code/price and save', async () => {
    vi.mocked(getOfficialSolution).mockResolvedValue(
      view({ access: 'creator', canEdit: true, solutionMd: 'old md', solutionCode: 'old code' }),
    )
    vi.mocked(updateOfficialSolution).mockResolvedValue(
      view({ access: 'creator', canEdit: true, solutionMd: 'new md', solutionCode: 'new code' }),
    )
    const wrapper = mountView()
    await flushPromises()

    await button(wrapper, '编辑官方题解')!.trigger('click')
    await flushPromises()

    await wrapper.find('.markdown-editor-mock').setValue('new md')
    await wrapper.find('textarea.code-textarea').setValue('new code')
    await wrapper.find('.input-number-mock').setValue('250')
    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(updateOfficialSolution).toHaveBeenCalledWith(1, {
      solutionMd: 'new md',
      solutionCode: 'new code',
      priceCoins: 250,
    })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('purchases with confirm and refreshes auth', async () => {
    vi.mocked(getOfficialSolution).mockResolvedValue(
      view({ access: 'locked', priceCoins: 100, canPurchase: true }),
    )
    vi.mocked(purchaseOfficialSolution).mockResolvedValue(
      view({ access: 'purchased', priceCoins: 100, solutionMd: 'unlocked body' }),
    )
    const wrapper = mountView()
    await flushPromises()

    await button(wrapper, '支付 100 金币查看')!.trigger('click')
    expect(confirm.require).toHaveBeenCalled()
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      acceptLabel?: string
      rejectLabel?: string
      accept?: () => void
    }
    expect(options.header).toBe('解锁官方题解')
    expect(options.message).toContain('100 金币')
    expect(options.acceptLabel).toBe('支付')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()

    expect(purchaseOfficialSolution).toHaveBeenCalledWith(1)
    expect(auth.fetchMe).toHaveBeenCalled()
    expect(wrapper.text()).toContain('unlocked body')
    wrapper.unmount()
  })
})
