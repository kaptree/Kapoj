import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Button from 'primevue/button'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { nextTick } from 'vue'

import { verifyCertificate } from '@/api/certificates'
import { ApiError } from '@/api/http'
import type { PublicCertificateView } from '@/api/types'
import CertificateVerifyView from './CertificateVerifyView.vue'

vi.mock('@/api/certificates', () => ({
  listCertificateTypes: vi.fn(),
  listMyCertificates: vi.fn(),
  submitCertificate: vi.fn(),
  getCertificateImage: vi.fn(),
  verifyCertificate: vi.fn(),
}))

function publicView(overrides: Partial<PublicCertificateView> = {}): PublicCertificateView {
  return {
    valid: true,
    type: '蓝桥杯',
    level: '省一',
    nickname: 'Alice',
    approvedAt: '2026-11-02T08:00:00Z',
    ...overrides,
  }
}

async function mountPage(query = '') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/verify', name: 'certificate-verify', component: { template: '<div />' } }],
  })
  await router.push(`/verify${query}`)
  await router.isReady()
  const wrapper = mount(CertificateVerifyView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  vi.mocked(verifyCertificate).mockReset()
})

describe('CertificateVerifyView', () => {
  it('verifies a code and renders the valid result without an image', async () => {
    vi.mocked(verifyCertificate).mockResolvedValue(publicView())
    const { wrapper } = await mountPage()

    await wrapper.find('input').setValue('KAPOJ-ABCD-EFGH')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(verifyCertificate).toHaveBeenCalledWith('KAPOJ-ABCD-EFGH')
    expect(wrapper.text()).toContain('验证有效')
    expect(wrapper.text()).toContain('蓝桥杯')
    expect(wrapper.text()).toContain('省一')
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('通过日期')
    expect(wrapper.find('img').exists()).toBe(false)
  })

  it('shows a neutral prompt for an invalid certificate', async () => {
    vi.mocked(verifyCertificate).mockResolvedValue({ valid: false })
    const { wrapper } = await mountPage()

    await wrapper.find('input').setValue('WRONG')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('未找到有效证书，请检查验证码')
    expect(wrapper.text()).not.toContain('验证有效')
  })

  it('auto-verifies from the ?code query param', async () => {
    vi.mocked(verifyCertificate).mockResolvedValue(publicView())
    const { wrapper } = await mountPage('?code=KAPOJ-ABCD-EFGH')

    expect(verifyCertificate).toHaveBeenCalledWith('KAPOJ-ABCD-EFGH')
    expect(wrapper.text()).toContain('验证有效')
  })

  it('does not call the API for an empty code', async () => {
    const { wrapper } = await mountPage()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(verifyCertificate).not.toHaveBeenCalled()
  })

  it('shows a loading state while verifying', async () => {
    let resolve!: (value: PublicCertificateView) => void
    vi.mocked(verifyCertificate).mockReturnValue(
      new Promise<PublicCertificateView>((r) => (resolve = r)),
    )
    const { wrapper } = await mountPage()

    await wrapper.find('input').setValue('KAPOJ-ABCD-EFGH')
    await wrapper.find('form').trigger('submit')
    await nextTick()

    expect(wrapper.findComponent(Button).props('loading')).toBe(true)

    resolve(publicView())
    await flushPromises()
    expect(wrapper.findComponent(Button).props('loading')).toBe(false)
    expect(wrapper.text()).toContain('验证有效')
  })

  it('shows an error when verification fails', async () => {
    vi.mocked(verifyCertificate).mockRejectedValue(new ApiError(500, 50000, '核验失败'))
    const { wrapper } = await mountPage()

    await wrapper.find('input').setValue('KAPOJ-ABCD-EFGH')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('核验失败')
    expect(wrapper.text()).not.toContain('验证有效')
  })
})
