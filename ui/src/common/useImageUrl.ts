import { useEffect, useState } from 'react'

type CacheEntry = {
  blobUrl: string | null
  error?: boolean
  refCount: number
}

type QueuedFetch = () => void

// Persists across component mount/unmount cycles so that
// React Admin refreshes (which remount list items) don't re-fetch images.
const cache = new Map<string, CacheEntry>()
const MAX_CACHE_SIZE = 300

// Limit concurrent fetches to leave browser connections free for API requests.
// Browsers allow ~6 connections per origin on HTTP/1.1; reserving 2 for API
// calls prevents image fetches from blocking pagination/data requests.
const MAX_CONCURRENT = 4
let activeFetches = 0
const pendingQueue: QueuedFetch[] = []

const processQueue = () => {
  while (pendingQueue.length > 0 && activeFetches < MAX_CONCURRENT) {
    pendingQueue.shift()?.()
  }
}

// Evicts oldest unused entries (Map iterates in insertion order).
const evictIfNeeded = () => {
  if (cache.size <= MAX_CACHE_SIZE) return
  for (const [key, entry] of cache) {
    if (cache.size <= MAX_CACHE_SIZE) break
    if (entry.refCount === 0) {
      if (entry.blobUrl) URL.revokeObjectURL(entry.blobUrl)
      cache.delete(key)
    }
  }
}

/**
 * Loads an image via fetch() with AbortController so that in-flight requests
 * are canceled on unmount (e.g., during pagination). Uses a module-level cache
 * so remounting returns the cached blob URL instantly.
 */
export const useImageUrl = (url?: string | null) => {
  const cached = url ? cache.get(url) : null
  const [imgUrl, setImgUrl] = useState(cached?.blobUrl || null)
  const [loading, setLoading] = useState(!!url && !cached)
  const [error, setError] = useState(cached?.error || false)

  useEffect(() => {
    let cancelled = false
    let retainedEntry: CacheEntry | undefined

    if (!url) {
      setImgUrl(null)
      setLoading(false)
      setError(false)
      return
    }

    // Re-check: another component's effect may have populated the cache
    // between this component's render and effect execution.
    const entry = cache.get(url)
    if (entry) {
      entry.refCount++
      retainedEntry = entry
      setImgUrl(entry.blobUrl)
      setLoading(false)
      setError(entry.error || false)
      return () => {
        entry.refCount = Math.max(0, entry.refCount - 1)
      }
    }

    const controller = new AbortController()
    let queued = true
    setImgUrl(null)
    setLoading(true)
    setError(false)

    const doFetch = () => {
      queued = false
      activeFetches++
      fetch(url, { signal: controller.signal })
        .then((res) => {
          if (!res.ok) {
            throw new Error(`HTTP ${res.status}`)
          }
          return res.blob()
        })
        .then((blob) => {
          // Guard against late resolution after abort
          if (cancelled) {
            return
          }
          const objectUrl = URL.createObjectURL(blob)
          // Handle concurrent fetches: if another component already cached
          // this URL, use its entry and discard our blob.
          const existing = cache.get(url)
          if (existing && existing.blobUrl) {
            existing.refCount++
            retainedEntry = existing
            URL.revokeObjectURL(objectUrl)
            setImgUrl(existing.blobUrl)
          } else {
            const created = { blobUrl: objectUrl, refCount: 1 }
            cache.set(url, created)
            retainedEntry = created
            evictIfNeeded()
            setImgUrl(objectUrl)
          }
          setLoading(false)
        })
        .catch((err) => {
          if (cancelled || (err as Error).name === 'AbortError') {
            return // Expected on unmount or URL change
          }
          // Cache the error so repeated mounts don't re-fetch broken URLs
          const failed = { blobUrl: null, error: true, refCount: 1 }
          cache.set(url, failed)
          retainedEntry = failed
          evictIfNeeded()
          setError(true)
          setLoading(false)
        })
        .finally(() => {
          activeFetches = Math.max(0, activeFetches - 1)
          processQueue()
        })
    }

    if (activeFetches < MAX_CONCURRENT) {
      queued = false
      doFetch()
    } else {
      pendingQueue.push(doFetch)
    }

    return () => {
      cancelled = true
      if (queued) {
        // Remove from queue if not yet started
        const idx = pendingQueue.indexOf(doFetch)
        if (idx !== -1) pendingQueue.splice(idx, 1)
      } else {
        controller.abort()
      }
      if (retainedEntry) {
        retainedEntry.refCount = Math.max(0, retainedEntry.refCount - 1)
      }
    }
  }, [url])

  return { imgUrl, loading, error }
}
