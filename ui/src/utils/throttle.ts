export default function throttle<T extends (...args: never[]) => void>(
  fn: T,
  waitMs: number,
): (...args: Parameters<T>) => void {
  let lastCall = 0
  let timeout: ReturnType<typeof setTimeout> | null = null

  return (...args: Parameters<T>) => {
    const now = Date.now()
    const remaining = waitMs - (now - lastCall)

    if (remaining <= 0) {
      if (timeout) {
        clearTimeout(timeout)
        timeout = null
      }
      lastCall = now
      fn(...args)
      return
    }

    if (!timeout) {
      timeout = setTimeout(() => {
        lastCall = Date.now()
        timeout = null
        fn(...args)
      }, remaining)
    }
  }
}
