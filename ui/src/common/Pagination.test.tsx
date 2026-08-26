import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  type ListPaginationContextValue,
  useListPaginationContext,
} from 'react-admin'
import { Pagination } from './Pagination'

// Stub RA's Pagination while preserving its real contract: Pagination reads
// setPerPage from ListPaginationContext rather than accepting it as a prop.
vi.mock('react-admin', async () => {
  const React = await vi.importActual<typeof import('react')>('react')
  const ListPaginationContext = React.createContext<{
    setPerPage: (value: number) => void
  } | null>(null)
  return {
    ListPaginationContext,
    Pagination: () => {
      const pagination = React.useContext(ListPaginationContext)
      if (!pagination) throw new Error('pagination context is missing')
      return React.createElement(
        'button',
        { onClick: () => pagination.setPerPage(50) },
        'select 50',
      )
    },
    useListPaginationContext: vi.fn(),
  }
})

describe('Pagination', () => {
  let mockContext = vi.mocked(useListPaginationContext)
  let setPerPage = vi.fn<(value: number) => void>()

  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    setPerPage = vi.fn()
    mockContext = vi.mocked(useListPaginationContext)
  })

  const context = (resource?: string) =>
    ({
      resource,
      page: 1,
      perPage: 15,
      total: 0,
      isPending: false,
      setPage: vi.fn(),
      setPerPage,
    }) satisfies ListPaginationContextValue

  const selectPerPage = () => fireEvent.click(screen.getByText('select 50'))

  it('persists the page size chosen in the selector', () => {
    mockContext.mockReturnValue(context('song'))
    render(<Pagination />)
    selectPerPage()
    expect(localStorage.getItem('perPage.song')).toEqual('50')
  })

  it('still applies the change to the list', () => {
    mockContext.mockReturnValue(context('song'))
    render(<Pagination />)
    selectPerPage()
    expect(setPerPage).toHaveBeenCalledWith(50)
  })

  it('does not persist a page size the user did not select', () => {
    mockContext.mockReturnValue(context('song'))
    render(<Pagination />)
    expect(localStorage.getItem('perPage.song')).toBeNull()
  })

  it('does not persist without a resource in context', () => {
    mockContext.mockReturnValue(context())
    render(<Pagination />)
    selectPerPage()
    expect(localStorage.getItem('perPage.undefined')).toBeNull()
    expect(setPerPage).toHaveBeenCalledWith(50)
  })
})
