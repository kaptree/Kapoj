import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import {
  getCertificateImage,
  listCertificateTypes,
  listMyCertificates,
  submitCertificate,
} from '@/api/certificates'
import { ApiError } from '@/api/http'
import type { CertificateStatus, CertificateTypeView, CertificateView } from '@/api/types'
import CertificateSettingsView from './CertificateSettingsView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const createObjectURL = vi.hoisted(() => vi.fn(() => 'blob:mock'))
const revokeObjectURL = vi.hoisted(() => vi.fn())
const clipboardWrite = vi.hoisted(() => vi.fn())

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/api/certificates', () => ({
  listCertificateTypes: vi.fn(),
  listMyCertificates: vi.fn(),
  submitCertificate: vi.fn(),
  getCertificateImage: vi.fn(),
}))

function typeView(overrides: Partial<CertificateTypeView> = {}): CertificateTypeView {
  return {
    id: 1,
    name: '蓝桥杯',
    description: '蓝桥杯全国软件和信息技术专业人才大赛',
    isActive: true,
    sortOrder: 1,
    ...overrides,
  }
}

function certView(overrides: Partial<CertificateView> = {}): CertificateView {
  return {
    id: 1,
    type: '蓝桥杯',
    level: '省一',
    status: 'pending',
    imageUrl: '/api/certificates/1/image',
    rejectReason: '',
    verifyCode: '',
    submittedAt: '2026-11-01T08:00:00Z',
    reviewedAt: null,
    ...overrides,
  }
}

async function mountPage() {
  const wrapper = mount(CertificateSettingsView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

function button(wrapper: Awaited<ReturnType<typeof mountPage>>, label: string) {
  const found = wrapper.findAll('button').find((b) => b.text().includes(label))
  expect(found, `button "${label}"`).toBeTruthy()
  return found!
}

function dialog(): HTMLElement {
  return document.querySelector('.p-dialog') as HTMLElement
}

function dialogButton(label: string): HTMLButtonElement {
  return Array.from(dialog().querySelectorAll('button')).find(
    (b) => b.textContent?.trim() === label,
  ) as HTMLButtonElement
}

function fileLike(name: string, size: number, type: string): File {
  const f = new File(['x'], name, { type })
  Object.defineProperty(f, 'size', { value: size })
  return f
}

async function setDialogFile(file: File): Promise<void> {
  const input = document.querySelector('input[type="file"]') as HTMLInputElement
  Object.defineProperty(input, 'files', { configurable: true, value: [file] })
  input.dispatchEvent(new Event('change', { bubbles: true }))
  await nextTick()
}

beforeEach(() => {
  document.body.innerHTML = ''
  toast.add.mockReset()
  createObjectURL.mockReset()
  createObjectURL.mockReturnValue('blob:mock')
  revokeObjectURL.mockReset()
  clipboardWrite.mockReset()
  clipboardWrite.mockResolvedValue(undefined)
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: clipboardWrite },
  })

  vi.mocked(listCertificateTypes).mockReset()
  vi.mocked(listMyCertificates).mockReset()
  vi.mocked(submitCertificate).mockReset()
  vi.mocked(getCertificateImage).mockReset()
  vi.mocked(listCertificateTypes).mockResolvedValue([typeView()])
  vi.mocked(listMyCertificates).mockResolvedValue({ total: 0, items: [] })
  vi.mocked(getCertificateImage).mockResolvedValue(new Blob(['x'], { type: 'image/png' }))
})

describe('CertificateSettingsView', () => {
  it('loads types and certificates in parallel and renders the list', async () => {
    vi.mocked(listMyCertificates).mockResolvedValue({
      total: 1,
      items: [certView({ status: 'approved', verifyCode: 'KAPOJ-ABCD-EFGH' })],
    })
    const wrapper = await mountPage()

    expect(listCertificateTypes).toHaveBeenCalledTimes(1)
    expect(listMyCertificates).toHaveBeenCalledWith(1, 100)
    expect(wrapper.text()).toContain('蓝桥杯')
    expect(wrapper.text()).toContain('省一')
    expect(wrapper.text()).toContain('已通过')
    expect(wrapper.text()).toContain('KAPOJ-ABCD-EFGH')
  })

  it('shows the empty state when there are no certificates', async () => {
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('还没有证书')
  })

  it('shows an error state and retries', async () => {
    vi.mocked(listCertificateTypes).mockRejectedValue(new ApiError(500, 50000, '加载失败'))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('加载失败')

    vi.mocked(listCertificateTypes).mockResolvedValue([typeView()])
    await button(wrapper, '重试').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('还没有证书')
    expect(listCertificateTypes).toHaveBeenCalledTimes(2)
  })

  it('renders status tags and the reject reason', async () => {
    vi.mocked(listMyCertificates).mockResolvedValue({
      total: 3,
      items: [
        certView({ id: 1, status: 'pending' }),
        certView({ id: 2, status: 'approved', verifyCode: 'KAPOJ-ABCD-EFGH' }),
        certView({ id: 3, status: 'rejected', rejectReason: '图片不清晰' }),
      ],
    })
    const wrapper = await mountPage()

    const tags = wrapper.findAllComponents(Tag)
    expect(tags.map((t) => t.props('value'))).toEqual(['待审核', '已通过', '已驳回'])
    expect(tags.map((t) => t.props('severity'))).toEqual(['warn', 'success', 'danger'])
    expect(wrapper.text()).toContain('图片不清晰')
  })

  it('copies the verify link for an approved certificate', async () => {
    vi.mocked(listMyCertificates).mockResolvedValue({
      total: 1,
      items: [certView({ status: 'approved', verifyCode: 'KAPOJ-ABCD-EFGH' })],
    })
    const wrapper = await mountPage()

    await button(wrapper, '复制链接').trigger('click')
    await flushPromises()

    expect(clipboardWrite).toHaveBeenCalledWith(
      expect.stringContaining('/verify?code=KAPOJ-ABCD-EFGH'),
    )
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ summary: '已复制' }))
  })

  it('validates required fields before submitting', async () => {
    const wrapper = await mountPage()

    await button(wrapper, '申请认证').trigger('click')
    await flushPromises()
    await dialogButton('提交').click()
    await flushPromises()

    expect(dialog().textContent).toContain('请选择证书类型')
    expect(dialog().textContent).toContain('请填写等级')
    expect(dialog().textContent).toContain('请选择证书图片')
    expect(submitCertificate).not.toHaveBeenCalled()
  })

  it('rejects an oversized or unsupported image before submitting', async () => {
    const wrapper = await mountPage()

    await button(wrapper, '申请认证').trigger('click')
    await flushPromises()

    await setDialogFile(fileLike('big.png', 5 * 1024 * 1024 + 1, 'image/png'))
    expect(dialog().textContent).toContain('证书图片不能超过 5 MiB')

    await setDialogFile(fileLike('notes.txt', 1024, 'text/plain'))
    expect(dialog().textContent).toContain('请选择 PNG、JPEG 或 WebP 格式的图片')
    expect(submitCertificate).not.toHaveBeenCalled()
  })

  it('submits a valid certificate and refreshes the list', async () => {
    vi.mocked(listMyCertificates).mockResolvedValue({ total: 0, items: [] })
    vi.mocked(submitCertificate).mockResolvedValue(certView({ status: 'pending' }))
    const wrapper = await mountPage()

    await button(wrapper, '申请认证').trigger('click')
    await flushPromises()

    wrapper.findComponent(Select).vm.$emit('update:modelValue', 1)
    await wrapper.findComponent(InputText).setValue('  省一  ')
    const picked = fileLike('cert.png', 1024, 'image/png')
    await setDialogFile(picked)

    await dialogButton('提交').click()
    await flushPromises()

    expect(submitCertificate).toHaveBeenCalledWith({ typeId: 1, level: '省一', file: picked })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    expect(listMyCertificates).toHaveBeenCalledTimes(2)
  })

  it('loads certificate images into object URLs and revokes them on unmount', async () => {
    vi.mocked(listMyCertificates).mockResolvedValue({
      total: 1,
      items: [certView({ id: 7, status: 'pending' as CertificateStatus })],
    })
    vi.mocked(getCertificateImage).mockResolvedValue(new Blob(['img'], { type: 'image/png' }))
    const wrapper = await mountPage()

    expect(getCertificateImage).toHaveBeenCalledWith(7)
    expect(createObjectURL).toHaveBeenCalledTimes(1)
    const img = wrapper.find('img.cert-thumb')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe('blob:mock')

    wrapper.unmount()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock')
  })
})
