import * as React from 'react'
import { render, screen, cleanup } from '@testing-library/react'
import { LibrarySelectionField } from './LibrarySelectionField'
import { useInput, useTranslate, useRecordContext } from 'react-admin'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { SelectLibraryInput } from '../common/SelectLibraryInput'
import { mockFn, mockUseInputValue } from '../test-utils/vitest'

// Mock the react-admin hooks
vi.mock('react-admin', () => ({
  useInput: vi.fn(),
  useTranslate: vi.fn(),
  useRecordContext: vi.fn(),
}))

// Mock the SelectLibraryInput component
vi.mock('../common/SelectLibraryInput', () => ({
  SelectLibraryInput: vi.fn(() => <div data-testid="select-library-input" />),
}))

describe('<LibrarySelectionField />', () => {
  const mockOnChange = vi.fn()
  const defaultProps = mockUseInputValue({
    value: [],
    onChange: mockOnChange,
  })

  const mockTranslate = vi.fn((key) => key)

  beforeEach(() => {
    mockFn(useInput).mockReturnValue(defaultProps)
    mockFn(useTranslate).mockReturnValue(mockTranslate)
    mockFn(useRecordContext).mockReturnValue({})
    vi.mocked(SelectLibraryInput).mockClear()
  })

  afterEach(cleanup)

  it('should render field label from translations', () => {
    render(<LibrarySelectionField />)
    expect(screen.getByText('resources.user.fields.libraries')).not.toBeNull()
  })

  it('should render helper text from translations', () => {
    render(<LibrarySelectionField />)
    expect(
      screen.getByText('resources.user.helperTexts.libraries'),
    ).not.toBeNull()
  })

  it('should render SelectLibraryInput with correct props', () => {
    render(<LibrarySelectionField />)
    expect(screen.getByTestId('select-library-input')).not.toBeNull()
    expect(SelectLibraryInput).toHaveBeenCalledWith(
      expect.objectContaining({
        onChange: mockOnChange,
        value: [],
      }),
      undefined,
    )
  })

  it('should render error message when touched and has error', () => {
    mockFn(useInput).mockReturnValue(
      mockUseInputValue({
        value: [],
        onChange: mockOnChange,
        error: 'This field is required',
        isTouched: true,
      }),
    )

    render(<LibrarySelectionField />)
    expect(screen.getByText('This field is required')).not.toBeNull()
  })

  it('should not render error message when not touched', () => {
    mockFn(useInput).mockReturnValue(
      mockUseInputValue({
        value: [],
        onChange: mockOnChange,
        error: 'This field is required',
        isTouched: false,
      }),
    )

    render(<LibrarySelectionField />)
    expect(screen.queryByText('This field is required')).toBeNull()
  })

  it('should initialize with empty array when value is null', () => {
    mockFn(useInput).mockReturnValue(
      mockUseInputValue({
        value: null,
        onChange: mockOnChange,
      }),
    )

    render(<LibrarySelectionField />)
    expect(SelectLibraryInput).toHaveBeenCalledWith(
      expect.objectContaining({
        value: [],
      }),
      undefined,
    )
  })

  it('should extract library IDs from record libraries array when editing user', () => {
    mockFn(useRecordContext).mockReturnValue({
      id: 'user123',
      name: 'John Doe',
      libraries: [
        { id: '1', name: 'Music Library 1', path: '/music1' },
        { id: '3', name: 'Music Library 3', path: '/music3' },
      ],
    })

    mockFn(useInput).mockReturnValue(
      mockUseInputValue({
        value: undefined,
        onChange: mockOnChange,
      }),
    )

    render(<LibrarySelectionField />)
    expect(SelectLibraryInput).toHaveBeenCalledWith(
      expect.objectContaining({
        value: ['1', '3'],
      }),
      undefined,
    )
  })

  it('should prefer libraryIds when both libraryIds and libraries are present', () => {
    mockFn(useRecordContext).mockReturnValue({
      id: 'user123',
      libraries: [
        { id: '1', name: 'Music Library 1', path: '/music1' },
        { id: '3', name: 'Music Library 3', path: '/music3' },
      ],
    })

    mockFn(useInput).mockReturnValue(
      mockUseInputValue({
        value: ['2', '4'],
        onChange: mockOnChange,
      }),
    )

    render(<LibrarySelectionField />)
    expect(SelectLibraryInput).toHaveBeenCalledWith(
      expect.objectContaining({
        value: ['2', '4'],
      }),
      undefined,
    )
  })
})
