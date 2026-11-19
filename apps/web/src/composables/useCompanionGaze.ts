import { onBeforeUnmount, ref, watch, type Ref } from 'vue'

export interface GazeRect {
  left: number
  top: number
  width: number
  height: number
}

export interface GazeVector {
  x: number
  y: number
}

/** Map a pointer position to a clamped -1..1 gaze vector. */
export function gazeVector(pointerX: number, pointerY: number, rect: GazeRect): GazeVector {
  if (rect.width === 0 || rect.height === 0) return { x: 0, y: 0 }
  const centreX = rect.left + rect.width / 2
  const centreY = rect.top + rect.height / 2
  const x = (pointerX - centreX) / (rect.width / 2)
  const y = (pointerY - centreY) / (rect.height / 2)
  return {
    x: Math.max(-1, Math.min(1, x)),
    y: Math.max(-1, Math.min(1, y)),
  }
}

/**
 * Track the window pointer and expose a smoothed, clamped gaze vector. The
 * composable is paused when `enabled` is false and cleans up every listener on
 * unmount.
 */
export function useCompanionGaze(options: {
  target: Ref<HTMLElement | null>
  enabled: Ref<boolean>
}) {
  const lookX = ref(0)
  const lookY = ref(0)
  let frame = 0
  let pending: { x: number; y: number } | null = null
  let started = false

  function reset(): void {
    lookX.value = 0
    lookY.value = 0
    pending = null
    if (frame) {
      cancelAnimationFrame(frame)
      frame = 0
    }
  }

  function apply(): void {
    frame = 0
    if (!pending || !options.enabled.value) return
    const element = options.target.value
    if (!element) return
    const vector = gazeVector(pending.x, pending.y, element.getBoundingClientRect())
    lookX.value = vector.x
    lookY.value = vector.y
  }

  function onPointerMove(event: Event): void {
    if (!options.enabled.value) return
    const pointer = event as MouseEvent
    pending = { x: pointer.clientX, y: pointer.clientY }
    if (!frame) frame = requestAnimationFrame(apply)
  }

  function onBlur(): void {
    reset()
  }

  function onVisibility(): void {
    if (document.hidden) reset()
  }

  function start(): void {
    if (started) return
    started = true
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerleave', reset)
    window.addEventListener('blur', onBlur)
    document.addEventListener('visibilitychange', onVisibility)
  }

  function stop(): void {
    if (!started) return
    started = false
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerleave', reset)
    window.removeEventListener('blur', onBlur)
    document.removeEventListener('visibilitychange', onVisibility)
    reset()
  }

  watch(
    options.enabled,
    (enabled) => {
      if (enabled) start()
      else stop()
    },
    { immediate: true },
  )

  onBeforeUnmount(stop)

  return { lookX, lookY, start, stop }
}
