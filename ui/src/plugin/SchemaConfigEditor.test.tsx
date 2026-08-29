import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import {
  ThemeProvider,
  StyledEngineProvider,
  createTheme,
} from '@mui/material/styles'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { SchemaConfigEditor } from './SchemaConfigEditor'
import { TestContext } from '../test/TestContext'

const theme = createTheme()

const renderWithProviders = (component) => {
  return render(
    <TestContext>
      <StyledEngineProvider injectFirst>
        <ThemeProvider theme={theme}>{component}</ThemeProvider>
      </StyledEngineProvider>
    </TestContext>,
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

  const basicUiSchema = {
    type: 'VerticalLayout',
    elements: [
      { type: 'Control', scope: '#/properties/name' },
      { type: 'Control', scope: '#/properties/enabled' },
    ],
  }

  it('renders nothing when schema is null', () => {
    const { container } = renderWithProviders(
      <SchemaConfigEditor schema={null} data={{}} uiSchema={{}} onChange={vi.fn()} />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders the component wrapper with valid schema', () => {
    const { container } = renderWithProviders(
      <SchemaConfigEditor schema={basicSchema} data={{}} uiSchema={basicUiSchema} onChange={vi.fn()} />,
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
        uiSchema={basicUiSchema}
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
        uiSchema={basicUiSchema}
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
