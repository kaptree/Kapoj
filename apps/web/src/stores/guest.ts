import { defineStore } from 'pinia'
import { ref } from 'vue'

import { getGuestPermissions } from '@/api/guest'

/**
 * Public guest permission set.
 *
 * Defaults to empty so the guest UI stays locked down until the public endpoint
 * confirms what the built-in guest role may see. A failed request also settles
 * on empty and marks the store loaded so a broken backend cannot trigger a
 * request storm on every navigation.
 */
export const useGuestStore = defineStore('guest', () => {
  const permissions = ref<string[]>([])
  const loaded = ref(false)

  let inFlight: Promise<void> | null = null

  async function load() {
    try {
      const result = await getGuestPermissions()
      permissions.value = result.permissions ?? []
    } catch {
      permissions.value = []
    } finally {
      loaded.value = true
    }
  }

  /** Loads guest permissions once; concurrent callers share the same request. */
  function ensureLoaded(): Promise<void> {
    if (loaded.value) return Promise.resolve()
    if (!inFlight) {
      inFlight = load().finally(() => {
        inFlight = null
      })
    }
    return inFlight
  }

  function can(code: string): boolean {
    return permissions.value.includes(code)
  }

  return { permissions, loaded, ensureLoaded, can }
})
