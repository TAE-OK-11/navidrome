import React from 'react'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import HotCacheAdmin from './HotCacheAdmin'

const usePermissions = vi.fn()

vi.mock('react-admin', () => ({
  Title: () => null,
  useNotify: () => vi.fn(),
  usePermissions: () => usePermissions(),
  useTranslate: () => (key) => key,
}))

describe('HotCacheAdmin authorization', () => {
  beforeEach(() =>
    usePermissions.mockReturnValue({ permissions: 'regular', loading: false }),
  )

  it('redirects non-administrators', () => {
    render(
      <MemoryRouter initialEntries={['/admin/hot-cache']}>
        <Route exact path="/admin/hot-cache">
          <HotCacheAdmin />
        </Route>
        <Route exact path="/">
          <div>library-home</div>
        </Route>
      </MemoryRouter>,
    )
    expect(screen.getByText('library-home')).toBeInTheDocument()
  })
})
