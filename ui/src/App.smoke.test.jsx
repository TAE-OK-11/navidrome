import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'

describe('App startup', () => {
  beforeEach(() => {
    localStorage.clear()
    window.location.hash = '#/'
  })
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('redirects a fresh profile and submits entered credentials', async () => {
    const NativeRequest = globalThis.Request
    vi.stubGlobal(
      'Request',
      class extends NativeRequest {
        constructor(input, init) {
          super(new URL(input, window.location.href), init)
        }
      },
    )
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      status: 401,
      statusText: 'Unauthorized',
    })
    render(<App />)

    expect(
      await screen.findByText('Navidrome', {}, { timeout: 10_000 }),
    ).toBeInTheDocument()
    expect(screen.queryByTestId('client-error-message')).not.toBeInTheDocument()

    const user = userEvent.setup()
    await user.type(screen.getByLabelText('Username'), 'listener')
    await user.type(screen.getByLabelText('Password'), 'secret')
    await user.click(screen.getByRole('button', { name: 'Sign in' }))

    await waitFor(() => expect(fetchSpy).toHaveBeenCalledOnce())
    const request = fetchSpy.mock.calls[0][0]
    await expect(request.json()).resolves.toEqual({
      username: 'listener',
      password: 'secret',
    })
    expect(screen.queryByText('Required')).not.toBeInTheDocument()
  }, 15_000)
})
