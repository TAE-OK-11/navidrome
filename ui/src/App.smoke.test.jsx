import React from 'react'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import App from './App'

describe('App startup', () => {
  beforeEach(() => {
    localStorage.clear()
    window.location.hash = '#/'
  })

  it('redirects a fresh browser profile to the login page', async () => {
    render(<App />)

    expect(
      await screen.findByText('Navidrome', {}, { timeout: 10_000 }),
    ).toBeInTheDocument()
    expect(screen.queryByTestId('client-error-message')).not.toBeInTheDocument()
  })
})
