import { renderHook } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { useAlbumsPerPage } from './useAlbumsPerPage'
import { setStoredPerPage } from './perPageStore'

describe('useAlbumsPerPage', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('falls back to the stored value on fresh load', () => {
    setStoredPerPage('album', 72)
    const { result } = renderHook(() => useAlbumsPerPage('lg'))
    expect(result.current[0]).toEqual(72)
  })

  it('ignores stored values invalid for the current width', () => {
    setStoredPerPage('album', 72) // valid for lg, not for md
    const { result } = renderHook(() => useAlbumsPerPage('md'))
    expect(result.current[0]).toEqual(12)
  })

  it('returns the responsive default when nothing is stored', () => {
    const { result } = renderHook(() => useAlbumsPerPage('xl'))
    expect(result.current).toEqual([36, [18, 36, 72]])
  })
})
