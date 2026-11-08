import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Avatar from 'primevue/avatar'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import { deleteAvatar, uploadAvatar } from '@/api/users'
import type { Me } from '@/api/types'
import ProfileEditView from './ProfileEditView.vue'

const baseUser: Me = {
  id: 1,
  username: 'alice',
  nickname: 'Alice',
  email: 'alice@example.com',
  avatarUrl: '',
  avatarUploaded: false,
  bio: 'Hello',
  role: 'user',
  roles: [],
  permissions: [],
  isSuperAdmin: false,
  rating: 1500,
  coins: 12,
  status: 'active',
  createdAt: '2026-01-01T00:00:00Z',
}

const auth = vi.hoisted(() => ({
  user: null as Me | null,
  fetchMe: vi.fn(),
  updateProfile: vi.fn(),
  setUser: vi.fn(),
}))

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const createObjectURL = vi.hoisted(() => vi.fn(() => 'blob:mock'))
const revokeObjectURL = vi.hoisted(() => vi.fn())

vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/api/users', () => ({ uploadAvatar: vi.fn(), deleteAvatar: vi.fn() }))

function fileLike(name: string, size: number, type: string): File {
  const file = new File(['x'], name, { type })
  Object.defineProperty(file, 'size', { value: size })
  return file
}

async function mountProfile() {
  auth.fetchMe.mockResolvedValue(undefined)
  const wrapper = mount(ProfileEditView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

function button(wrapper: Awaited<ReturnType<typeof mountProfile>>, label: string) {
  const found = wrapper.findAll('button').find((item) => item.text().includes(label))
  expect(found, `button "${label}"`).toBeTruthy()
  return found!
}

async function setFile(wrapper: Awaited<ReturnType<typeof mountProfile>>, file: File) {
  const input = wrapper.find('input[type="file"]')
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
}

const avatarInput = (wrapper: Awaited<ReturnType<typeof mountProfile>>) =>
  wrapper.findAll('input')[1]!

beforeEach(() => {
  auth.user = { ...baseUser }
  auth.fetchMe.mockReset()
  auth.updateProfile.mockReset()
  auth.setUser.mockReset()
  auth.setUser.mockImplementation((me: Me) => {
    auth.user = me
  })
  toast.add.mockReset()
  vi.mocked(uploadAvatar).mockReset()
  vi.mocked(deleteAvatar).mockReset()
  createObjectURL.mockReset()
  createObjectURL.mockReturnValue('blob:mock')
  revokeObjectURL.mockReset()
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })
})

describe('ProfileEditView', () => {
  it('fills the form from the current user', async () => {
    const wrapper = await mountProfile()
    const inputs = wrapper.findAll('input')

    expect((inputs[0]!.element as HTMLInputElement).value).toBe('Alice')
    expect((inputs[1]!.element as HTMLInputElement).value).toBe('')
    expect((wrapper.find('textarea').element as HTMLTextAreaElement).value).toBe('Hello')
  })

  it('does not submit a blank nickname or an invalid avatar URL', async () => {
    const wrapper = await mountProfile()
    const inputs = wrapper.findAll('input')

    await inputs[0]!.setValue('   ')
    await inputs[1]!.setValue('ftp://example.com/avatar.png')
    await wrapper.find('form').trigger('submit')

    expect(wrapper.text()).toContain('请输入昵称')
    expect(wrapper.text()).toContain('头像地址需为 http 或 https 链接')
    expect(auth.updateProfile).not.toHaveBeenCalled()
  })

  it('saves the profile through auth.updateProfile', async () => {
    auth.updateProfile.mockResolvedValue(undefined)
    const wrapper = await mountProfile()

    await wrapper.findAll('input')[0]!.setValue('Alice2')
    await wrapper.find('textarea').setValue('New bio')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(auth.updateProfile).toHaveBeenCalledWith({
      nickname: 'Alice2',
      bio: 'New bio',
      avatarUrl: '',
    })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
  })

  it('keeps the form usable and reports save failures', async () => {
    auth.updateProfile.mockRejectedValue(new ApiError(500, 50000, '保存失败，请稍后再试'))
    const wrapper = await mountProfile()

    await wrapper.findAll('input')[0]!.setValue('Alice2')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'error', detail: '保存失败，请稍后再试' }),
    )
    expect(wrapper.text()).toContain('保存失败，请稍后再试')
  })

  it('keeps the external field blank and previews the internal URL for an uploaded avatar', async () => {
    auth.user = {
      ...baseUser,
      avatarUploaded: true,
      avatarUrl: '/api/users/1/avatar?v=123',
    }
    const wrapper = await mountProfile()

    expect(avatarInput(wrapper).element.value).toBe('')
    expect(wrapper.findComponent(Avatar).props('image')).toBe('/api/users/1/avatar?v=123')
    expect(wrapper.text()).toContain('填写外链会替换已上传头像')
  })

  it('preserves an external avatar URL in the field', async () => {
    auth.user = { ...baseUser, avatarUrl: 'https://example.com/a.png' }
    const wrapper = await mountProfile()

    expect(avatarInput(wrapper).element.value).toBe('https://example.com/a.png')
  })

  it('warns when uploading without a file and never calls the API', async () => {
    const wrapper = await mountProfile()

    await button(wrapper, '上传头像').trigger('click')
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'warn' }))
    expect(uploadAvatar).not.toHaveBeenCalled()
  })

  it('rejects an oversize file and a non-image file with a toast', async () => {
    const wrapper = await mountProfile()

    await setFile(wrapper, fileLike('big.png', 3 * 1024 * 1024, 'image/png'))
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ summary: '头像不能超过 2 MiB' }),
    )
    expect(uploadAvatar).not.toHaveBeenCalled()

    await setFile(wrapper, fileLike('notes.txt', 10, 'text/plain'))
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ summary: '仅支持 PNG、JPEG、WebP 或 GIF 图片' }),
    )
    expect(uploadAvatar).not.toHaveBeenCalled()
  })

  it('uploads a valid file, updates the auth user, clears the field and toasts', async () => {
    const file = fileLike('avatar.png', 100, 'image/png')
    const uploadedMe: Me = {
      ...baseUser,
      avatarUploaded: true,
      avatarUrl: '/api/users/1/avatar?v=999',
    }
    vi.mocked(uploadAvatar).mockResolvedValue(uploadedMe)
    const wrapper = await mountProfile()

    await setFile(wrapper, file)
    expect(wrapper.findComponent(Avatar).props('image')).toBe('blob:mock')

    await button(wrapper, '上传头像').trigger('click')
    await flushPromises()

    expect(uploadAvatar).toHaveBeenCalledWith(file)
    expect(auth.setUser).toHaveBeenCalledWith(uploadedMe)
    expect(avatarInput(wrapper).element.value).toBe('')
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '头像已上传' }),
    )
    expect(wrapper.findComponent(Avatar).props('image')).toBe('/api/users/1/avatar?v=999')
  })

  it('removes the avatar, updates the auth user, clears the field and toasts', async () => {
    auth.user = {
      ...baseUser,
      avatarUploaded: true,
      avatarUrl: '/api/users/1/avatar?v=123',
    }
    const removedMe: Me = { ...baseUser, avatarUploaded: false, avatarUrl: '' }
    vi.mocked(deleteAvatar).mockResolvedValue(removedMe)
    const wrapper = await mountProfile()

    await button(wrapper, '移除头像').trigger('click')
    await flushPromises()

    expect(deleteAvatar).toHaveBeenCalled()
    expect(auth.setUser).toHaveBeenCalledWith(removedMe)
    expect(avatarInput(wrapper).element.value).toBe('')
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '头像已移除' }),
    )
  })

  it('saves an empty external URL when an avatar is uploaded (backend preserves it)', async () => {
    auth.user = { ...baseUser, avatarUploaded: true, avatarUrl: '/api/users/1/avatar?v=123' }
    auth.updateProfile.mockResolvedValue(undefined)
    const wrapper = await mountProfile()

    await wrapper.findAll('input')[0]!.setValue('Alice2')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(auth.updateProfile).toHaveBeenCalledWith(expect.objectContaining({ avatarUrl: '' }))
  })

  it('saves a non-empty external URL that replaces the uploaded avatar', async () => {
    auth.user = { ...baseUser, avatarUploaded: true, avatarUrl: '/api/users/1/avatar?v=123' }
    auth.updateProfile.mockResolvedValue(undefined)
    const wrapper = await mountProfile()

    await avatarInput(wrapper).setValue('https://example.com/new.png')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(auth.updateProfile).toHaveBeenCalledWith(
      expect.objectContaining({ avatarUrl: 'https://example.com/new.png' }),
    )
  })

  it('revokes the pending object URL when the component unmounts', async () => {
    const wrapper = await mountProfile()

    await setFile(wrapper, fileLike('avatar.png', 100, 'image/png'))
    expect(createObjectURL).toHaveBeenCalledTimes(1)

    wrapper.unmount()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock')
  })
})
