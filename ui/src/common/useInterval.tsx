// From https://overreacted.io/making-setinterval-declarative-with-react-hooks/

import { useEffect, useRef } from 'react'

export const useInterval = (callback: () => void, delay: number | null) => {
  const savedCallback = useRef(callback)

  // Remember the latest callback.
  useEffect(() => {
    savedCallback.current = callback
  }, [callback])

  // Set up the interval.
  useEffect(() => {
    const tick = () => {
      savedCallback.current()
    }
    if (delay !== null) {
      const id = window.setInterval(tick, delay)
      return () => clearInterval(id)
    }
  }, [delay])
}
