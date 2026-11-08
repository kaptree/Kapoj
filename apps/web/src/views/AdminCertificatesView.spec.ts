import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createAdminCertificateType,
  deleteAdminCertificateType,
  listAdminCertificates,
  listAdminCertificateTypes,
  reviewCertificate,
  updateAdminCertificateType,
} from '@/api/admin'
import { getCertificateImage } from '@/api/certificates'
import { ApiError } from '@/api/http'
import type { AdminCertificateView, CertificateTypeView } from '@/api/types'
import AdminCertificatesView from './AdminCertificatesView.vue'

const PAGE_SIZE = 20

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const createObjectURL = vi.hoisted(() => vi.fn(() => 'blob:mock'))
const revokeObjectURL = vi.hoisted(() => vi.fn())

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/admin', () => ({
  listAdminCertificates: vi.fn(),
  reviewCertificate: vi.fn(),
  listAdminCertificateTypes: vi.fn(),
  createAdminCertificateType: vi.fn(),
  updateAdminCertificateType: vi.fn(),
  deleteAdminCertificateType: vi.fn(),
}))
vi.mock('@/api/certificates', () => ({ getCertificateImage: vi.fn() }))

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

function certView(overrides: Partial<AdminCertificateView> = {}): AdminCertificateView {
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
    user: { id: 10, username: 'alice', nickname: 'Alice' },
    ...overrides,
  }
}

async function mountPage() {
  const wrapper = mount(AdminCertificatesView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

function tab(wrapper: Awaited<ReturnType<typeof mountPage>>, label: string) {
  const found = wrapper.findAll('[role="tab"]').find((t) => t.text() === label)
  expect(found, `tab "${label}"`).toBeTruthy()
  return found!
}

function button(wrapper: Awaited<ReturnType<typeof mountPage>>, label: string) {
  const found = wrapper.findAll('button').find((b) => b.text().trim() === label)
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

beforeEach(() => {
  document.body.innerHTML = ''
  toast.add.mockReset()
  confirm.require.mockReset()
  createObjectURL.mockReset()
  createObjectURL.mockReturnValue('blob:mock')
  revokeObjectURL.mockReset()
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })

  vi.mocked(listAdminCertificates).mockReset()
  vi.mocked(reviewCertificate).mockReset()
  vi.mocked(listAdminCertificateTypes).mockReset()
  vi.mocked(createAdminCertificateType).mockReset()
  vi.mocked(updateAdminCertificateType).mockReset()
  vi.mocked(deleteAdminCertificateType).mockReset()
  vi.mocked(getCertificateImage).mockReset()

  vi.mocked(listAdminCertificates).mockResolvedValue({ total: 0, items: [] })
  vi.mocked(listAdminCertificateTypes).mockResolvedValue([])
  vi.mocked(getCertificateImage).mockResolvedValue(new Blob(['x'], { type: 'image/png' }))
})

describe('AdminCertificatesView', () => {
  it('loads the pending review list by default', async () => {
    vi.mocked(listAdminCertificates).mockResolvedValue({ total: 1, items: [certView()] })
    const wrapper = await mountPage()

    expect(listAdminCertificates).toHaveBeenCalledWith({
      status: 'pending',
      page: 1,
      pageSize: PAGE_SIZE,
    })
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('蓝桥杯')
    expect(wrapper.text()).toContain('省一')
    expect(wrapper.text()).toContain('待审核')
    wrapper.unmount()
  })

  it('filters by status and paginates', async () => {
    vi.mocked(listAdminCertificates).mockResolvedValue({
      total: 25,
      items: [certView(), certView({ id: 2 })],
    })
    const wrapper = await mountPage()

    await wrapper.findComponent(Select).vm.$emit('update:modelValue', 'all')
    await flushPromises()
    expect(listAdminCertificates).toHaveBeenLastCalledWith({
      status: 'all',
      page: 1,
      pageSize: PAGE_SIZE,
    })

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(listAdminCertificates).toHaveBeenLastCalledWith({
      status: 'all',
      page: 2,
      pageSize: PAGE_SIZE,
    })
    wrapper.unmount()
  })

  it('shows the review list empty state and load errors', async () => {
    let wrapper = await mountPage()
    expect(wrapper.text()).toContain('暂无证书')
    wrapper.unmount()

    vi.mocked(listAdminCertificates).mockRejectedValue(new ApiError(403, -1, '没有权限'))
    wrapper = await mountPage()
    expect(wrapper.text()).toContain('没有权限')
    wrapper.unmount()
  })

  it('loads the certificate image as a blob URL and revokes it on unmount', async () => {
    vi.mocked(listAdminCertificates).mockResolvedValue({ total: 1, items: [certView({ id: 7 })] })
    const wrapper = await mountPage()

    await button(wrapper, '审核').trigger('click')
    await flushPromises()

    expect(getCertificateImage).toHaveBeenCalledWith(7)
    expect(createObjectURL).toHaveBeenCalledTimes(1)
    const img = dialog().querySelector('img.review-image')
    expect(img).toBeTruthy()
    expect(img!.getAttribute('src')).toBe('blob:mock')

    wrapper.unmount()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock')
  })

  it('approves a certificate, toasts, and reloads', async () => {
    vi.mocked(listAdminCertificates).mockResolvedValue({ total: 1, items: [certView()] })
    vi.mocked(reviewCertificate).mockResolvedValue(certView({ status: 'approved' }))
    const wrapper = await mountPage()

    await button(wrapper, '审核').trigger('click')
    await flushPromises()
    await dialogButton('通过').click()
    await flushPromises()

    expect(reviewCertificate).toHaveBeenCalledWith(1, { action: 'approve' })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已通过' }),
    )
    expect(listAdminCertificates).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('blocks reject without a reason and rejects with one', async () => {
    vi.mocked(listAdminCertificates).mockResolvedValue({ total: 1, items: [certView()] })
    vi.mocked(reviewCertificate).mockResolvedValue(certView({ status: 'rejected' }))
    const wrapper = await mountPage()

    await button(wrapper, '审核').trigger('click')
    await flushPromises()

    await dialogButton('驳回').click()
    await flushPromises()
    expect(reviewCertificate).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('请填写驳回原因')

    await wrapper.findComponent(Textarea).setValue('  图片不清晰  ')
    await dialogButton('驳回').click()
    await flushPromises()

    expect(reviewCertificate).toHaveBeenCalledWith(1, { action: 'reject', reason: '图片不清晰' })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已驳回' }),
    )
    expect(listAdminCertificates).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('lists certificate types on the types tab', async () => {
    vi.mocked(listAdminCertificateTypes).mockResolvedValue([
      typeView(),
      typeView({ id: 2, name: 'NOIP', description: '', isActive: false, sortOrder: 2 }),
    ])
    const wrapper = await mountPage()

    await tab(wrapper, '证书类型').trigger('click')
    await flushPromises()

    expect(listAdminCertificateTypes).toHaveBeenCalledWith(true)
    expect(wrapper.text()).toContain('蓝桥杯')
    expect(wrapper.text()).toContain('NOIP')
    wrapper.unmount()
  })

  it('validates the type form before creating', async () => {
    vi.mocked(listAdminCertificateTypes).mockResolvedValue([])
    const wrapper = await mountPage()
    await tab(wrapper, '证书类型').trigger('click')
    await flushPromises()

    await button(wrapper, '新建类型').trigger('click')
    await flushPromises()

    await dialogButton('保存').click()
    await flushPromises()
    expect(createAdminCertificateType).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('请填写类型名称')

    await wrapper.findComponent(InputText).setValue('x'.repeat(51))
    await dialogButton('保存').click()
    await flushPromises()
    expect(dialog().textContent).toContain('类型名称不能超过 50 字')

    await wrapper.findComponent(InputText).setValue('NOIP')
    await wrapper.findComponent(Textarea).setValue('x'.repeat(201))
    await dialogButton('保存').click()
    await flushPromises()
    expect(dialog().textContent).toContain('类型描述不能超过 200 字')

    await wrapper.findComponent(Textarea).setValue('')
    await wrapper.findComponent(InputNumber).vm.$emit('update:modelValue', 10001)
    await dialogButton('保存').click()
    await flushPromises()
    expect(dialog().textContent).toContain('排序需为 0–10000 的整数')
    expect(createAdminCertificateType).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('creates a type and reloads', async () => {
    vi.mocked(listAdminCertificateTypes).mockResolvedValue([])
    vi.mocked(createAdminCertificateType).mockResolvedValue(typeView({ id: 5, name: 'NOIP' }))
    const wrapper = await mountPage()
    await tab(wrapper, '证书类型').trigger('click')
    await flushPromises()

    await button(wrapper, '新建类型').trigger('click')
    await flushPromises()

    await wrapper.findComponent(InputText).setValue('  NOIP  ')
    await wrapper.findComponent(Textarea).setValue('  全国青少年信息学奥林匹克联赛  ')
    await wrapper.findComponent(InputNumber).vm.$emit('update:modelValue', 2)
    await dialogButton('保存').click()
    await flushPromises()

    expect(createAdminCertificateType).toHaveBeenCalledWith({
      name: 'NOIP',
      description: '全国青少年信息学奥林匹克联赛',
      isActive: true,
      sortOrder: 2,
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '类型已创建' }),
    )
    expect(listAdminCertificateTypes).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('edits a type', async () => {
    vi.mocked(listAdminCertificateTypes).mockResolvedValue([typeView()])
    vi.mocked(updateAdminCertificateType).mockResolvedValue(typeView({ name: 'NOIP' }))
    const wrapper = await mountPage()
    await tab(wrapper, '证书类型').trigger('click')
    await flushPromises()

    await button(wrapper, '编辑').trigger('click')
    await flushPromises()
    expect((dialog().querySelector('.name-input') as HTMLInputElement).value).toBe('蓝桥杯')

    await wrapper.findComponent(InputText).setValue('NOIP')
    await wrapper.findComponent(ToggleSwitch).vm.$emit('update:modelValue', false)
    await dialogButton('保存').click()
    await flushPromises()

    expect(updateAdminCertificateType).toHaveBeenCalledWith(1, {
      name: 'NOIP',
      description: '蓝桥杯全国软件和信息技术专业人才大赛',
      isActive: false,
      sortOrder: 1,
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '类型已更新' }),
    )
    wrapper.unmount()
  })

  it('deletes a type on confirm and reloads', async () => {
    vi.mocked(listAdminCertificateTypes).mockResolvedValue([typeView()])
    vi.mocked(deleteAdminCertificateType).mockResolvedValue(null)
    const wrapper = await mountPage()
    await tab(wrapper, '证书类型').trigger('click')
    await flushPromises()

    await button(wrapper, '删除').trigger('click')
    const options = confirm.require.mock.calls[0]![0] as { header?: string; accept?: () => void }
    expect(options.header).toBe('删除类型')
    options.accept?.()
    await flushPromises()

    expect(deleteAdminCertificateType).toHaveBeenCalledWith(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '类型已删除' }),
    )
    expect(listAdminCertificateTypes).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('surfaces an in-use error when deleting a referenced type', async () => {
    vi.mocked(listAdminCertificateTypes).mockResolvedValue([typeView()])
    vi.mocked(deleteAdminCertificateType).mockRejectedValue(
      new ApiError(400, 40001, '已有证书使用该类型，请先停用'),
    )
    const wrapper = await mountPage()
    await tab(wrapper, '证书类型').trigger('click')
    await flushPromises()

    await button(wrapper, '删除').trigger('click')
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(deleteAdminCertificateType).toHaveBeenCalledWith(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        detail: '已有证书使用该类型，请先停用',
      }),
    )
    wrapper.unmount()
  })
})
