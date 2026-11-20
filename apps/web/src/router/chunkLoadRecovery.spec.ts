import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Mock } from 'vitest'

import { installChunkLoadRecovery, isChunkLoadError } from './chunkLoadRecovery'

type ErrorListener = (error: unknown) => void

function fakeRouter() {
  const listeners = new Set<ErrorListener>()
  return {
    onError: vi.fn((listener: ErrorListener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    }),
    emitError(error: unknown) {
      for (const listener of listeners) listener(error)
    },
  }
}

function memoryStorage() {
  const values = new Map<string, string>()
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value)
    },
  }
}

describe('isChunkLoadError', () => {
  it.each([
    new TypeError('Failed to fetch dynamically imported module: /assets/SubmissionsView-old.js'),
    new Error('error loading dynamically imported module'),
    new Error('Importing a module script failed.'),
    new Error('Unable to preload CSS for /assets/SubmissionsView-old.css'),
    Object.assign(new Error('Loading chunk 12 failed'), { name: 'ChunkLoadError' }),
  ])('recognizes stale build errors: %s', (error) => {
    expect(isChunkLoadError(error)).toBe(true)
  })

  it('ignores unrelated runtime errors', () => {
    expect(isChunkLoadError(new Error('Network request failed'))).toBe(false)
    expect(isChunkLoadError('something else')).toBe(false)
    expect(isChunkLoadError(undefined)).toBe(false)
  })
})

describe('installChunkLoadRecovery', () => {
  let reload: Mock<() => void>
  let now: number

  beforeEach(() => {
    reload = vi.fn<() => void>()
    now = 1_000
  })

  it('reloads once when Vite cannot preload a stale chunk', () => {
    const router = fakeRouter()
    installChunkLoadRecovery(router, {
      reload,
      storage: memoryStorage(),
      now: () => now,
    })

    const event = new Event('vite:preloadError', { cancelable: true })
    Object.defineProperty(event, 'payload', {
      value: new TypeError('Failed to fetch dynamically imported module: /assets/old.js'),
    })
    window.dispatchEvent(event)

    expect(reload).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)
  })

  it('ignores preload errors that are not stale chunks', () => {
    const router = fakeRouter()
    installChunkLoadRecovery(router, {
      reload,
      storage: memoryStorage(),
      now: () => now,
    })

    const event = new Event('vite:preloadError', { cancelable: true })
    Object.defineProperty(event, 'payload', { value: new Error('network down') })
    window.dispatchEvent(event)

    expect(reload).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })

  it('reloads when the router reports a dynamic import failure', () => {
    const router = fakeRouter()
    installChunkLoadRecovery(router, {
      reload,
      storage: memoryStorage(),
      now: () => now,
    })

    router.emitError(new TypeError('Failed to fetch dynamically imported module: /assets/old.js'))

    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('does not reload repeatedly within the guard window', () => {
    const router = fakeRouter()
    installChunkLoadRecovery(router, {
      reload,
      storage: memoryStorage(),
      now: () => now,
    })

    router.emitError(new TypeError('Failed to fetch dynamically imported module: /assets/old.js'))
    now += 5_000
    router.emitError(new TypeError('Failed to fetch dynamically imported module: /assets/old.js'))

    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('allows another reload after the guard window', () => {
    const router = fakeRouter()
    installChunkLoadRecovery(router, {
      reload,
      storage: memoryStorage(),
      now: () => now,
    })

    router.emitError(new TypeError('Failed to fetch dynamically imported module: /assets/old.js'))
    now += 16_000
    router.emitError(new TypeError('Failed to fetch dynamically imported module: /assets/old.js'))

    expect(reload).toHaveBeenCalledTimes(2)
  })

  it('stops listening when disposed', () => {
    const router = fakeRouter()
    const dispose = installChunkLoadRecovery(router, {
      reload,
      storage: memoryStorage(),
      now: () => now,
    })

    dispose()
    router.emitError(new TypeError('Failed to fetch dynamically imported module: /assets/old.js'))
    window.dispatchEvent(
      Object.assign(new Event('vite:preloadError', { cancelable: true }), {
        payload: new TypeError('Failed to fetch dynamically imported module: /assets/old.js'),
      }),
    )

    expect(reload).not.toHaveBeenCalled()
  })
})
