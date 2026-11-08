import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import { listFollowers, listFollowing } from '@/api/social'
import type { SocialUserSummary } from '@/api/types'
import FollowListDialog from './FollowListDialog.vue'

vi.mock('@/api/social', () => ({
  listFollowers: vi.fn(),
  listFollowing: vi.fn(),
}))

function user(overrides: Partial<SocialUserSummary> = {}): SocialUserSummary {
  return {
    id: 2,
    username: 'bob',
    nickname: 'Bob',
    avatarUrl: '',
    rating: 1500,
    ...overrides,
  }
}

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(FollowListDialog, {
    props: { userId: 7, visible: true, ...props },
    global: {
      plugins: [PrimeVue],
      stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } },
    },
  })
}

function tabByLabel(wrapper: ReturnType<typeof mountDialog>, label: string) {
  return wrapper.findAll('.p-tab').find((t) => t.text().trim() === label)
}

beforeEach(() => {
  document.body.innerHTML = ''
  vi.mocked(listFollowers).mockReset()
  vi.mocked(listFollowing).mockReset()
})

describe('FollowListDialog', () => {
  it('loads followers by default and lists users with rating links', async () => {
    vi.mocked(listFollowers).mockResolvedValue({
      total: 1,
      items: [user({ nickname: 'Bob', rating: 1500 })],
    })
    const wrapper = mountDialog()
    await flushPromises()

    expect(listFollowers).toHaveBeenCalledWith(7, { page: 1, pageSize: 20 })
    expect(wrapper.text()).toContain('Bob')
    expect(wrapper.text()).toContain('@bob')
    expect(wrapper.find('a[href="/users/2"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('switches to the following tab and loads following', async () => {
    vi.mocked(listFollowers).mockResolvedValue({ total: 0, items: [] })
    vi.mocked(listFollowing).mockResolvedValue({
      total: 1,
      items: [user({ nickname: 'Alice' })],
    })
    const wrapper = mountDialog()
    await flushPromises()

    await tabByLabel(wrapper, '关注')!.trigger('click')
    await flushPromises()

    expect(listFollowing).toHaveBeenCalledWith(7, { page: 1, pageSize: 20 })
    expect(wrapper.text()).toContain('Alice')
    wrapper.unmount()
  })

  it('shows an empty state when there are no followers', async () => {
    vi.mocked(listFollowers).mockResolvedValue({ total: 0, items: [] })
    const wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.text()).toContain('暂无粉丝')
    wrapper.unmount()
  })

  it('shows an error and retries', async () => {
    vi.mocked(listFollowers)
      .mockRejectedValueOnce(new ApiError(500, 50000, '服务器错误'))
      .mockResolvedValueOnce({ total: 1, items: [user()] })
    const wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.text()).toContain('服务器错误')
    const retry = wrapper.findAll('button').find((b) => b.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()

    expect(listFollowers).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Bob')
    wrapper.unmount()
  })

  it('paginates through the follower list', async () => {
    vi.mocked(listFollowers).mockResolvedValue({ total: 45, items: [user()] })
    const wrapper = mountDialog()
    await flushPromises()

    wrapper.findComponent({ name: 'Paginator' }).vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(listFollowers).toHaveBeenLastCalledWith(7, { page: 2, pageSize: 20 })
    wrapper.unmount()
  })
})
