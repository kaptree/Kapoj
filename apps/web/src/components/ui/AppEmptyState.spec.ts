import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { useCompanionStore } from '@/stores/companion'
import AppEmptyState from './AppEmptyState.vue'

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

describe('AppEmptyState', () => {
  it('renders the companion mascot and line when provided', () => {
    const wrapper = mount(AppEmptyState, {
      props: { title: '暂无记录', mascot: 'lost', companionLine: '这里什么都没有。' },
    })
    expect(wrapper.find('.companion-avatar__image').exists()).toBe(true)
    expect(wrapper.text()).toContain('这里什么都没有。')
  })

  it('keeps the icon when no mascot is set', () => {
    const wrapper = mount(AppEmptyState, { props: { title: '空', icon: 'pi pi-inbox' } })
    expect(wrapper.find('.pi-inbox').exists()).toBe(true)
  })

  it('hides the mascot when the companion is disabled', () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    const wrapper = mount(AppEmptyState, { props: { title: '空', mascot: 'lost' } })
    expect(wrapper.find('.companion-avatar').exists()).toBe(false)
    expect(wrapper.find('.app-empty__icon').exists()).toBe(true)
  })

  it('hides the mascot when the site disables the companion globally', () => {
    useCompanionStore().applyGlobalConfig({
      enabled: false,
      name: 'Kapo',
      defaultActivityLevel: 'normal',
      contestMute: true,
      allowUserDisable: true,
    })
    const wrapper = mount(AppEmptyState, { props: { title: '空', mascot: 'lost' } })
    expect(wrapper.find('.companion-avatar').exists()).toBe(false)
  })

  it('drops the icon box while the mascot is visible', () => {
    const wrapper = mount(AppEmptyState, { props: { title: '空', mascot: 'lost' } })
    expect(wrapper.find('.app-empty__icon').exists()).toBe(false)
  })

  it('renders the companion line only alongside a mascot', () => {
    const wrapper = mount(AppEmptyState, {
      props: { title: '空', companionLine: '这里什么都没有。' },
    })
    expect(wrapper.text()).not.toContain('这里什么都没有。')
  })

  it('keeps rendering without an active Pinia', () => {
    setActivePinia(undefined)
    const wrapper = mount(AppEmptyState, { props: { title: '空', mascot: 'lost' } })
    expect(wrapper.find('.companion-avatar__image').exists()).toBe(true)
  })
})
