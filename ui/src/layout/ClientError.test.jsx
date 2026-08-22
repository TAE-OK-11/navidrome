import React from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import ClientError from './ClientError'

describe('ClientError', () => {
  it('shows the original client error and can retry', () => {
    const resetErrorBoundary = vi.fn()
    render(
      <ClientError
        error={new Error('render failed')}
        resetErrorBoundary={resetErrorBoundary}
      />,
    )

    expect(screen.getByTestId('client-error-message')).toHaveTextContent(
      'render failed',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(resetErrorBoundary).toHaveBeenCalledOnce()
  })
})
