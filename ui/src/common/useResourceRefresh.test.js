import { renderHook } from '@testing-library/react'
import { vi } from 'vitest'
import * as Redux from 'react-redux'
import * as RA from 'react-admin'
import { useResourceRefresh } from './useResourceRefresh'

vi.mock('react-redux', async () => {
  const actual = await vi.importActual('react-redux')
  return { ...actual, useSelector: vi.fn() }
})

vi.mock('react-admin', async () => {
  const actual = await vi.importActual('react-admin')
  return {
    ...actual,
    useRefresh: vi.fn(),
    useDataProvider: vi.fn(),
  }
})

describe('useResourceRefresh', () => {
  const refresh = vi.fn()
  const getMany = vi.fn()
  let refreshData

  beforeEach(() => {
    vi.mocked(RA.useRefresh).mockReturnValue(refresh)
    vi.mocked(RA.useDataProvider).mockReturnValue({ getMany })
    vi.mocked(Redux.useSelector).mockImplementation((selector) =>
      selector({ activity: { refresh: refreshData } }),
    )
    refreshData = undefined
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  const renderRefreshHook = (...resources) =>
    renderHook(() => useResourceRefresh(...resources))

  it('does not repeat work for the same event timestamp', () => {
    refreshData = {
      lastReceived: Date.now() + 1000,
      resources: { album: ['al-1'] },
    }
    const { rerender } = renderRefreshHook()
    rerender()

    expect(getMany).toHaveBeenCalledOnce()
  })

  it('triggers a UI refresh for a global resource event', () => {
    refreshData = {
      lastReceived: Date.now() + 1000,
      resources: { '*': '*' },
    }
    renderRefreshHook()

    expect(refresh).toHaveBeenCalledOnce()
    expect(getMany).not.toHaveBeenCalled()
  })

  it('triggers a UI refresh for a wildcard resource id', () => {
    refreshData = {
      lastReceived: Date.now() + 1000,
      resources: { album: ['*'] },
    }
    renderRefreshHook()

    expect(refresh).toHaveBeenCalledOnce()
    expect(getMany).not.toHaveBeenCalled()
  })

  it('refetches every received resource when no filter is specified', () => {
    refreshData = {
      lastReceived: Date.now() + 1000,
      resources: { album: ['al-1', 'al-2'], song: ['sg-1', 'sg-2'] },
    }
    renderRefreshHook()

    expect(refresh).not.toHaveBeenCalled()
    expect(getMany).toHaveBeenCalledTimes(2)
    expect(getMany).toHaveBeenCalledWith('album', { ids: ['al-1', 'al-2'] })
    expect(getMany).toHaveBeenCalledWith('song', { ids: ['sg-1', 'sg-2'] })
  })

  it('refetches only visible resources when a filter is specified', () => {
    refreshData = {
      lastReceived: Date.now() + 1000,
      resources: { album: ['al-1', 'al-2'], song: ['sg-1', 'sg-2'] },
    }
    renderRefreshHook('song')

    expect(refresh).not.toHaveBeenCalled()
    expect(getMany).toHaveBeenCalledOnce()
    expect(getMany).toHaveBeenCalledWith('song', { ids: ['sg-1', 'sg-2'] })
  })
})
