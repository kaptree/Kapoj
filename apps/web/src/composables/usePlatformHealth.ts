import { onBeforeUnmount, onMounted, readonly, ref } from 'vue'

export type PlatformHealthState = 'checking' | 'ok' | 'degraded' | 'unreachable' | 'offline'

interface HealthEnvelope {
  data?: {
    status?: string
  }
}

/**
 * Reads the public /health endpoint so the home status panel reflects the
 * server instead of a hard-coded "ready" label. It re-checks when the browser
 * comes back online; it never polls in the background.
 */
export function usePlatformHealth() {
  const state = ref<PlatformHealthState>('checking')
  let latest = 0
  let disposed = false

  async function check() {
    if (typeof navigator !== 'undefined' && !navigator.onLine) {
      latest += 1
      state.value = 'offline'
      return
    }

    const current = ++latest
    state.value = 'checking'
    try {
      const response = await fetch('/health', {
        cache: 'no-store',
        headers: { Accept: 'application/json' },
      })
      const body = (await response.json().catch(() => null)) as HealthEnvelope | null
      if (current !== latest || disposed) return
      state.value = body?.data?.status === 'ok' ? 'ok' : 'degraded'
    } catch {
      if (current !== latest || disposed) return
      state.value = 'unreachable'
    }
  }

  function handleOnline() {
    void check()
  }

  function handleOffline() {
    latest += 1
    state.value = 'offline'
  }

  onMounted(() => {
    void check()
    window.addEventListener('online', handleOnline)
    window.addEventListener('offline', handleOffline)
  })

  onBeforeUnmount(() => {
    disposed = true
    latest += 1
    window.removeEventListener('online', handleOnline)
    window.removeEventListener('offline', handleOffline)
  })

  return {
    state: readonly(state),
    check,
  }
}
