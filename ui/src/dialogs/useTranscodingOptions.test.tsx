import React from 'react'
import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useGetList } from 'react-admin'
import { useTranscodingOptions } from './useTranscodingOptions'

vi.mock('react-admin', () => ({
  BooleanInput: () => null,
  SelectInput: () => null,
  useGetList: vi.fn(),
  useTranslate: () => (key) => key,
}))

describe('useTranscodingOptions', () => {
  beforeEach(() => vi.clearAllMocks())

  it('handles the initial react-admin 5 loading state', () => {
    vi.mocked(useGetList).mockReturnValue({ data: undefined, isPending: true } as any)

    expect(() => renderHook(() => useTranscodingOptions())).not.toThrow()
    expect(useGetList).toHaveBeenCalledWith('transcoding', {
      pagination: { page: 1, perPage: 1000 },
      sort: { field: 'name', order: 'ASC' },
      filter: {},
    })
  })

  it('accepts the array returned by useGetList', () => {
    vi.mocked(useGetList).mockReturnValue({
      data: [{ targetFormat: 'opus', name: 'Opus' }],
      isPending: false,
    } as any)

    const { result } = renderHook(() => useTranscodingOptions())

    expect(result.current.TranscodingOptionsInput).toBeTypeOf('function')
  })
})
