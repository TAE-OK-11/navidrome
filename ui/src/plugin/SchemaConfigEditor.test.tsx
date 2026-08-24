// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import {
  ThemeProvider,
  StyledEngineProvider,
  createTheme,
} from '@mui/material/styles'
import { Provider } from 'react-redux'
import { createStore } from 'redux'
import { SchemaConfigEditor } from './SchemaConfigEditor'

const theme = createTheme()

// JSONForms requires Redux
const mockStore = createStore(() => ({}))

const renderWithProviders = (component) => {
  return render(
    <Provider store={mockStore}>
      <StyledEngineProvider injectFirst>
        <ThemeProvider theme={theme}>{component}</ThemeProvider>
      </StyledEngineProvider>
    </Provider>,
  )
}

describe('SchemaConfigEditor', () => {
  const basicSchema = {
    type: 'object',
    properties: {
      name: {
        type: 'string',
        title: 'Name',
      },
      enabled: {
        type: 'boolean',
        title: 'Enabled',
      },
    },
  }

  it('renders nothing when schema is null', () => {
    const { container } = renderWithProviders(
      <SchemaConfigEditor schema={null} data={{}} onChange={vi.fn()} />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders the component wrapper with valid schema', () => {
    const { container } = renderWithProviders(
      <SchemaConfigEditor schema={basicSchema} data={{}} onChange={vi.fn()} />,
    )
    // Check that the wrapper div is rendered (class name is generated)
    expect(
      container.querySelector('[class*="NDSchemaConfigEditor-root"]'),
    ).toBeTruthy()
  })

  it('does not emit a synthetic change on initial render', () => {
    const onChange = vi.fn()
    renderWithProviders(
      <SchemaConfigEditor
        schema={basicSchema}
        data={{ name: 'Test' }}
        onChange={onChange}
      />,
    )

    expect(onChange).not.toHaveBeenCalled()
  })

  it('passes data and errors to onChange callback', async () => {
    const onChange = vi.fn()
    const initialData = { name: 'Test Value' }

    renderWithProviders(
      <SchemaConfigEditor
        schema={basicSchema}
        data={initialData}
        onChange={onChange}
      />,
    )

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Changed Value' },
    })

    await waitFor(() =>
      expect(onChange).toHaveBeenCalledWith(
        expect.objectContaining({ name: 'Changed Value' }),
        expect.any(Array),
      ),
    )
  })
})
