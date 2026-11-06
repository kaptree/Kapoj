import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import type { Me } from '@/api/types'
import ProfileEditView from './ProfileEditView.vue'

const baseUser: Me = {
  id: 1,
  username: 'alice',
  nickname: 'Alice',
  email: 'alice@example.com',
  avatarUrl: '',
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
  setUser: vi.fn(),
}))

const usersApi = vi.hoisted(() => ({
  updateMe: vi.fn(),
}))

const toast = vi.hoisted(() => ({
  add: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

vi.mock('@/api/users', () => ({
  updateMe: usersApi.updateMe,
}))

vi.mock('primevue/usetoast', () => ({
  useToast: () => toast,
}))

async function mountProfile() {
  auth.fetchMe.mockResolvedValue(undefined)
  const wrapper = mount(ProfileEditView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  auth.user = { ...baseUser }
  auth.fetchMe.mockReset()
  auth.setUser.mockReset()
  usersApi.updateMe.mockReset()
  toast.add.mockReset()
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
    expect(usersApi.updateMe).not.toHaveBeenCalled()
  })

  it('saves the profile and updates the auth user', async () => {
    const updated = { ...baseUser, nickname: 'Alice2', bio: 'New bio' }
    usersApi.updateMe.mockResolvedValue(updated)
    const wrapper = await mountProfile()

    await wrapper.findAll('input')[0]!.setValue('Alice2')
    await wrapper.find('textarea').setValue('New bio')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(usersApi.updateMe).toHaveBeenCalledWith({
      nickname: 'Alice2',
      bio: 'New bio',
      avatarUrl: '',
    })
    expect(auth.setUser).toHaveBeenCalledWith(updated)
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
  })

  it('keeps the form usable and reports save failures', async () => {
    usersApi.updateMe.mockRejectedValue(new ApiError(500, 50000, '保存失败，请稍后再试'))
    const wrapper = await mountProfile()

    await wrapper.findAll('input')[0]!.setValue('Alice2')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(auth.setUser).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'error', detail: '保存失败，请稍后再试' }),
    )
    expect(wrapper.text()).toContain('保存失败，请稍后再试')
  })
})
