import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import HotCacheAdmin from './HotCacheAdmin'

const usePermissions = vi.fn()
const hotCacheApi = vi.hoisted(() => ({
  getHotCacheDashboard: vi.fn(),
  getHotCacheEntries: vi.fn(),
  getHotCacheCandidates: vi.fn(),
  hotCacheAction: vi.fn(),
  promoteHotCacheCandidates: vi.fn(),
}))

vi.mock('react-admin', () => ({
  Title: () => null,
  useNotify: () => vi.fn(),
  usePermissions: () => usePermissions(),
  useTranslate: () => (key) => key,
}))

vi.mock('./api', () => hotCacheApi)

describe('HotCacheAdmin authorization', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    usePermissions.mockReturnValue({
      permissions: 'regular',
      loading: false,
    })
    hotCacheApi.getHotCacheDashboard.mockResolvedValue({
      status: {},
      sessions: [],
      queue: [],
      current: null,
      formats: [],
      events: [],
      errors: [],
      artwork: [],
    })
  })

  it('redirects non-administrators', () => {
    render(
      <MemoryRouter initialEntries={['/admin/hot-cache']}>
        <Routes>
          <Route path="/admin/hot-cache" element={<HotCacheAdmin />} />
          <Route path="/" element={<div>library-home</div>} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('library-home')).toBeInTheDocument()
  })

  it('loads the administrator dashboard through one aggregate request', async () => {
    usePermissions.mockReturnValue({ permissions: 'admin', loading: false })
    render(
      <MemoryRouter initialEntries={['/admin/hot-cache']}>
        <Routes>
          <Route path="/admin/hot-cache" element={<HotCacheAdmin />} />
        </Routes>
      </MemoryRouter>,
    )
    await waitFor(() =>
      expect(hotCacheApi.getHotCacheDashboard).toHaveBeenCalledTimes(1),
    )
    expect(hotCacheApi.getHotCacheEntries).not.toHaveBeenCalled()
    expect(hotCacheApi.getHotCacheCandidates).not.toHaveBeenCalled()
  })
})
