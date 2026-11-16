import { computed, getCurrentScope, onScopeDispose, ref, type ComputedRef } from 'vue'

import { useCompanionStore } from '@/stores/companion'

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

/**
 * True when the companion should avoid motion: either because the visitor chose
 * the reduced-motion preference or because the operating system asks for it.
 *
 * Works inside a component scope (listener cleaned up on dispose) and outside
 * one, so tests can mount it through a host component without leaking listeners.
 */
export function useReducedMotion(): ComputedRef<boolean> {
  const store = useCompanionStore()
  const mediaMatches = ref(false)

  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    const mediaQueryList = window.matchMedia(REDUCED_MOTION_QUERY)
    mediaMatches.value = mediaQueryList.matches

    if (typeof mediaQueryList.addEventListener === 'function') {
      const update = (event: MediaQueryListEvent) => {
        mediaMatches.value = event.matches
      }

      mediaQueryList.addEventListener('change', update)
      if (getCurrentScope()) {
        onScopeDispose(() => mediaQueryList.removeEventListener('change', update))
      }
    }
  }

  return computed(() => store.motion === 'reduce' || mediaMatches.value)
}
