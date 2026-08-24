// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'

const compatibilityWarning =
  /adaptV4Theme|Invalid attribute name|ContentProps|InputProps|TransitionProps/

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
    const consoleError = vi.spyOn(console, 'error')
    const consoleWarn = vi.spyOn(console, 'warn')
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
    expect(
      [...consoleError.mock.calls, ...consoleWarn.mock.calls].filter((args) =>
        compatibilityWarning.test(args.join(' ')),
      ),
    ).toEqual([])
  }, 15_000)

  it('renders the first resource for an authenticated admin', async () => {
    const consoleError = vi.spyOn(console, 'error')
    const consoleWarn = vi.spyOn(console, 'warn')
    const payload = btoa(
      JSON.stringify({
        exp: Math.floor(Date.now() / 1000) + 3600,
        uid: 'admin-id',
        sub: 'admin',
      }),
    )
    localStorage.setItem('token', `eyJhbGciOiJIUzI1NiJ9.${payload}.test`)
    localStorage.setItem('userId', 'admin-id')
    localStorage.setItem('name', 'Admin')
    localStorage.setItem('username', 'admin')
    localStorage.setItem('role', 'admin')
    localStorage.setItem('is-authenticated', 'true')
    window.location.hash = '#/album/recent?initial=1'

    window.EventSource = class {
      addEventListener() {}
      close() {}
    }
    window.ResizeObserver = class {
      observe() {}
      disconnect() {}
    }

    const fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation((input) => {
        const url = String(input)
        let body = []
        if (/\/api\/user\/admin-id(?:\?|$)/.test(url)) {
          body = { id: 'admin-id', libraries: [] }
        } else if (url.includes('/rest/getScanStatus')) {
          body = {
            'subsonic-response': {
              status: 'ok',
              scanStatus: { scanning: false, count: 0 },
            },
          }
        }
        return Promise.resolve(
          new Response(JSON.stringify(body), {
            status: 200,
            headers: {
              'Content-Type': 'application/json',
              'X-Total-Count': Array.isArray(body) ? '0' : '1',
            },
          }),
        )
      })

    render(<App />)

    await waitFor(
      () =>
        expect(
          fetchSpy.mock.calls.some(([input]) =>
            String(input).includes('/api/album?'),
          ),
        ).toBe(true),
      { timeout: 10_000 },
    )
    expect(screen.queryByText('Something went wrong')).not.toBeInTheDocument()

    window.location.hash = '#/personal'
    expect(
      await screen.findAllByText('Default View', {}, { timeout: 10_000 }),
    ).not.toHaveLength(0)
    expect(screen.queryByText('Not Found')).not.toBeInTheDocument()
    expect(
      [...consoleError.mock.calls, ...consoleWarn.mock.calls].filter((args) =>
        compatibilityWarning.test(args.join(' ')),
      ),
    ).toEqual([])
  }, 15_000)
})
