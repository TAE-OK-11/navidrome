import { renderHook, act } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest'
import type { useImageUrl as UseImageUrlFn } from './useImageUrl'

const flushPromises = () => new Promise((resolve) => setTimeout(resolve, 0))

let useImageUrl: typeof UseImageUrlFn

describe('useImageUrl', () => {
  let abortSpy: ReturnType<typeof vi.fn>
  let OriginalAbortController: typeof AbortController
  let originalCreateObjectURL: typeof URL.createObjectURL
  let originalRevokeObjectURL: typeof URL.revokeObjectURL
  let originalFetch: typeof fetch

  beforeEach(async () => {
    vi.resetModules()
    const mod = await import('./useImageUrl')
    useImageUrl = mod.useImageUrl

    abortSpy = vi.fn()
    OriginalAbortController = globalThis.AbortController
    originalCreateObjectURL = globalThis.URL.createObjectURL
    originalRevokeObjectURL = globalThis.URL.revokeObjectURL
    originalFetch = globalThis.fetch

    class MockAbortController {
      signal = { aborted: false } as AbortSignal
      abort = abortSpy
    }
    globalThis.AbortController =
      MockAbortController as unknown as typeof AbortController
    globalThis.URL.createObjectURL = vi.fn(() => 'blob:mock-url')
    globalThis.URL.revokeObjectURL = vi.fn()
  })

  afterEach(() => {
    globalThis.AbortController = OriginalAbortController
    globalThis.URL.createObjectURL = originalCreateObjectURL
    globalThis.URL.revokeObjectURL = originalRevokeObjectURL
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('should return null values when url is null', () => {
    const { result } = renderHook(() => useImageUrl(null))

    expect(result.current.loading).toBe(false)
    expect(result.current.imgUrl).toBeNull()
    expect(result.current.error).toBe(false)
  })

  it('should return loading state initially', () => {
    globalThis.fetch = vi.fn(
      () => new Promise(() => {}),
    ) as unknown as typeof fetch
    const { result } = renderHook(() =>
      useImageUrl('http://example.com/img.jpg'),
    )

    expect(result.current.loading).toBe(true)
    expect(result.current.imgUrl).toBeNull()
    expect(result.current.error).toBe(false)
  })

  it('should fetch image and return blob URL on success', async () => {
    const mockBlob = new Blob(['image-data'], { type: 'image/png' })
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        blob: () => Promise.resolve(mockBlob),
      }),
    ) as unknown as typeof fetch

    const { result } = renderHook(() =>
      useImageUrl('http://example.com/img.jpg'),
    )

    await act(async () => {
      await flushPromises()
    })

    expect(result.current.loading).toBe(false)
    expect(result.current.imgUrl).toBe('blob:mock-url')
    expect(result.current.error).toBe(false)
    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://example.com/img.jpg',
      {
        signal: expect.anything(),
      },
    )
  })

  it('should set error on HTTP failure', async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({ ok: false, status: 404 }),
    ) as unknown as typeof fetch

    const { result } = renderHook(() =>
      useImageUrl('http://example.com/missing.jpg'),
    )

    await act(async () => {
      await flushPromises()
    })

    expect(result.current.loading).toBe(false)
    expect(result.current.imgUrl).toBeNull()
    expect(result.current.error).toBe(true)
  })

  it('should abort fetch on unmount', async () => {
    globalThis.fetch = vi.fn(
      () => new Promise(() => {}),
    ) as unknown as typeof fetch

    const { unmount } = renderHook(() =>
      useImageUrl('http://example.com/img.jpg'),
    )

    await act(async () => {
      await flushPromises()
    })

    unmount()

    expect(abortSpy).toHaveBeenCalled()
  })

  it('should abort previous fetch when URL changes', async () => {
    const abortSpies: Array<ReturnType<typeof vi.fn>> = []
    class ChangingAbortController {
      signal = { aborted: false } as AbortSignal
      abort: ReturnType<typeof vi.fn>
      constructor() {
        const spy = vi.fn()
        abortSpies.push(spy)
        this.abort = spy
      }
    }
    globalThis.AbortController =
      ChangingAbortController as unknown as typeof AbortController

    const mockBlob = new Blob(['data'], { type: 'image/png' })
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        blob: () => Promise.resolve(mockBlob),
      }),
    ) as unknown as typeof fetch

    const { rerender } = renderHook(({ url }) => useImageUrl(url), {
      initialProps: { url: 'http://example.com/img1.jpg' },
    })

    await act(async () => {
      await flushPromises()
    })

    rerender({ url: 'http://example.com/img2.jpg' })

    expect(abortSpies[0]).toHaveBeenCalled()
  })

  it('should not set error on AbortError', async () => {
    const abortError = new DOMException('Aborted', 'AbortError')
    globalThis.fetch = vi.fn(() =>
      Promise.reject(abortError),
    ) as unknown as typeof fetch

    const { result } = renderHook(() =>
      useImageUrl('http://example.com/img.jpg'),
    )

    await act(async () => {
      await flushPromises()
    })

    expect(result.current.error).toBe(false)
  })

  it('should use cached blob URL on remount without re-fetching', async () => {
    const mockBlob = new Blob(['data'], { type: 'image/png' })
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        blob: () => Promise.resolve(mockBlob),
      }),
    ) as unknown as typeof fetch

    const { unmount } = renderHook(() =>
      useImageUrl('http://example.com/img.jpg'),
    )

    await act(async () => {
      await flushPromises()
    })

    expect(globalThis.fetch).toHaveBeenCalledTimes(1)

    unmount()

    const { result: result2 } = renderHook(() =>
      useImageUrl('http://example.com/img.jpg'),
    )

    await act(async () => {
      await flushPromises()
    })

    expect(globalThis.fetch).toHaveBeenCalledTimes(1)
    expect(result2.current.imgUrl).toBe('blob:mock-url')
    expect(result2.current.loading).toBe(false)
  })

  it('should cache errors and not re-fetch broken URLs', async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({ ok: false, status: 404 }),
    ) as unknown as typeof fetch

    const { unmount } = renderHook(() =>
      useImageUrl('http://example.com/broken.jpg'),
    )

    await act(async () => {
      await flushPromises()
    })

    expect(globalThis.fetch).toHaveBeenCalledTimes(1)
    unmount()

    const { result: result2 } = renderHook(() =>
      useImageUrl('http://example.com/broken.jpg'),
    )

    await act(async () => {
      await flushPromises()
    })

    expect(globalThis.fetch).toHaveBeenCalledTimes(1)
    expect(result2.current.error).toBe(true)
    expect(result2.current.imgUrl).toBeNull()
    expect(result2.current.loading).toBe(false)
  })
})
