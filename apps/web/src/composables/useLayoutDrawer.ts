import { nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'

export type LayoutMenuButtonHandle = { $el?: HTMLElement }

/**
 * Shared mobile-drawer behavior for the site and admin shells.
 * The drawer closes on navigation and Escape, and restores focus to the
 * toggle button when it closes.
 */
export function useLayoutDrawer(menuButton: Ref<LayoutMenuButtonHandle | null> = ref(null)) {
  const route = useRoute()
  const menuOpen = ref(false)

  function focusMenuButton() {
    const element = menuButton.value?.$el
    if (element instanceof HTMLElement && element.offsetParent !== null) element.focus()
  }

  function openMenu() {
    menuOpen.value = true
  }

  function closeMenu(restoreFocus = true) {
    if (!menuOpen.value) return
    menuOpen.value = false
    if (restoreFocus) void nextTick(focusMenuButton)
  }

  function onDrawerHide() {
    void nextTick(focusMenuButton)
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && menuOpen.value) {
      event.preventDefault()
      closeMenu()
    }
  }

  onMounted(() => window.addEventListener('keydown', onKeydown))
  onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))

  watch(
    () => route.fullPath,
    () => closeMenu(false),
  )

  return { menuOpen, menuButton, openMenu, closeMenu, onDrawerHide }
}
