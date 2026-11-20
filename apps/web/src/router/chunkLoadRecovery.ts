const RELOAD_GUARD_KEY = 'kapoj:chunk-reload-at'
const RELOAD_GUARD_MS = 15_000

const CHUNK_ERROR_PATTERNS = [
  /failed to fetch dynamically imported module/i,
  /error loading dynamically imported module/i,
  /importing a module script failed/i,
  /unable to preload (?:css|module)/i,
  /loading chunk \d+ failed/i,
]

/** True when a lazy route or preloaded asset belongs to a build that is no longer deployed. */
export function isChunkLoadError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const { name, message } = error as { name?: unknown; message?: unknown }
  if (name === 'ChunkLoadError') return true
  if (typeof message !== 'string') return false
  return CHUNK_ERROR_PATTERNS.some((pattern) => pattern.test(message))
}

export interface ChunkRecoveryRouter {
  onError(handler: (error: unknown) => void): () => void
}

interface PreloadErrorEvent extends Event {
  payload?: unknown
}

interface ChunkRecoveryOptions {
  /** Test seam; production reloads the current document. */
  reload?: () => void
  /** Test seam; production reads the wall clock. */
  now?: () => number
  /** Test seam; production uses sessionStorage. */
  storage?: Pick<Storage, 'getItem' | 'setItem'>
}

function sessionStorageOrUndefined(): Pick<Storage, 'getItem' | 'setItem'> | undefined {
  try {
    return window.sessionStorage
  } catch {
    return undefined
  }
}

/**
 * Vite reports failed dynamic imports with `vite:preloadError`; Vue Router also
 * surfaces the same failure through its error hook. Both paths reload the page
 * so the browser fetches the current index.html and its hashed assets.
 */
export function installChunkLoadRecovery(
  router: ChunkRecoveryRouter,
  options: ChunkRecoveryOptions = {},
): () => void {
  const reload = options.reload ?? (() => window.location.reload())
  const now = options.now ?? (() => Date.now())
  const storage = options.storage ?? sessionStorageOrUndefined()

  function reloadOnce(): boolean {
    try {
      const raw = storage?.getItem(RELOAD_GUARD_KEY)
      if (raw !== null && raw !== undefined) {
        const previous = Number(raw)
        if (Number.isFinite(previous) && now() - previous < RELOAD_GUARD_MS) return false
      }
      storage?.setItem(RELOAD_GUARD_KEY, String(now()))
    } catch {
      // Private browsing can reject storage access; reloading is still preferable.
    }
    reload()
    return true
  }

  function onPreloadError(event: Event): void {
    const error = (event as PreloadErrorEvent).payload
    if (!isChunkLoadError(error)) return
    if (reloadOnce()) event.preventDefault()
  }

  const removeRouterHandler = router.onError((error) => {
    if (isChunkLoadError(error)) reloadOnce()
  })
  window.addEventListener('vite:preloadError', onPreloadError)

  return () => {
    window.removeEventListener('vite:preloadError', onPreloadError)
    removeRouterHandler()
  }
}
