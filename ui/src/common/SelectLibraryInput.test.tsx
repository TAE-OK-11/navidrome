import * as React from 'react'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import { SelectLibraryInput } from './SelectLibraryInput'
import { useGetList } from 'react-admin'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

// Mock Material-UI components
vi.mock('@mui/material', () => ({
  List: ({ children }) => <div>{children}</div>,
  ListItemButton: ({ children, onClick, className }) => (
    <button onClick={onClick} className={className}>
      {children}
    </button>
  ),
  ListItemIcon: ({ children }) => <span>{children}</span>,
  ListItemText: ({ primary }) => <span>{primary}</span>,
  Typography: ({ children, variant }) => <span>{children}</span>,
  Box: ({ children, className }) => <div className={className}>{children}</div>,
  Checkbox: ({
    checked,
    indeterminate,
    onChange,
    size,
    className,
    ...props
  }) => (
    <input
      type="checkbox"
      checked={checked}
      ref={(el) => {
        if (el) el.indeterminate = indeterminate
      }}
      onChange={onChange}
      className={className}
      {...props}
    />
  ),
  makeStyles: () => () => ({}),
}))

// Mock Material-UI icons
vi.mock('@mui/icons-material', () => ({
  CheckBox: () => <span>☑</span>,
  CheckBoxOutlineBlank: () => <span>☐</span>,
}))

// Mock the react-admin hook
vi.mock('react-admin', () => ({
  useGetList: vi.fn(),
  useTranslate: vi.fn(() => (key) => key), // Simple translation mock
}))

type LibraryRecord = { id: string; name: string; defaultNewUsers?: boolean }

const mockLibrariesResponse = (libraries: LibraryRecord[], isLoading = false) =>
  vi.mocked(useGetList).mockReturnValue({
    data: libraries,
    isLoading,
  } as unknown as ReturnType<typeof useGetList>)

describe('<SelectLibraryInput />', () => {
  const mockOnChange = vi.fn()

  beforeEach(() => {
    // Reset the mock before each test
    mockOnChange.mockClear()
  })

  afterEach(cleanup)

  it('should render empty message when no libraries available', () => {
    mockLibrariesResponse([])

    render(<SelectLibraryInput onChange={mockOnChange} value={[]} />)
    expect(screen.getByText('No libraries available')).not.toBeNull()
  })

  it('should render libraries when available', () => {
    const mockLibraries: LibraryRecord[] = [
      { id: '1', name: 'Library 1' },
      { id: '2', name: 'Library 2' },
    ]
    mockLibrariesResponse(mockLibraries)

    render(<SelectLibraryInput onChange={mockOnChange} value={[]} />)
    expect(screen.getByText('Library 1')).not.toBeNull()
    expect(screen.getByText('Library 2')).not.toBeNull()
  })

  it('should toggle selection when a library is clicked', () => {
    const mockLibraries: LibraryRecord[] = [
      { id: '1', name: 'Library 1' },
      { id: '2', name: 'Library 2' },
    ]

    mockLibrariesResponse(mockLibraries)
    render(<SelectLibraryInput onChange={mockOnChange} value={[]} />)

    const library1Button = screen.getByText('Library 1').closest('button')!
    fireEvent.click(library1Button)
    expect(mockOnChange).toHaveBeenCalledWith(['1'])

    cleanup()
    mockOnChange.mockClear()

    mockLibrariesResponse(mockLibraries)
    render(
      <SelectLibraryInput onChange={mockOnChange} value={['1'] as string[]} />,
    )

    const library1ButtonDeselect = screen
      .getByText('Library 1')
      .closest('button')!
    fireEvent.click(library1ButtonDeselect)
    expect(mockOnChange).toHaveBeenCalledWith([])
  })

  it('should correctly initialize with provided values', () => {
    const mockLibraries: LibraryRecord[] = [
      { id: '1', name: 'Library 1' },
      { id: '2', name: 'Library 2' },
    ]
    mockLibrariesResponse(mockLibraries)

    render(
      <SelectLibraryInput onChange={mockOnChange} value={['1'] as string[]} />,
    )

    const checkboxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    expect(checkboxes[1].checked).toBe(true)
    expect(checkboxes[2].checked).toBe(false)
  })

  it('should handle value as array of objects', () => {
    const mockLibraries: LibraryRecord[] = [
      { id: '1', name: 'Library 1' },
      { id: '2', name: 'Library 2' },
    ]
    mockLibrariesResponse(mockLibraries)

    render(
      <SelectLibraryInput
        onChange={mockOnChange}
        value={[{ id: '2' }] as Array<{ id: string }>}
      />,
    )

    const checkboxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    expect(checkboxes[1].checked).toBe(false)
    expect(checkboxes[2].checked).toBe(true)
  })

  it('should render master checkbox when there are multiple libraries', () => {
    const mockLibraries: LibraryRecord[] = [
      { id: '1', name: 'Library 1' },
      { id: '2', name: 'Library 2' },
    ]
    mockLibrariesResponse(mockLibraries)

    render(<SelectLibraryInput onChange={mockOnChange} value={[]} />)

    const checkboxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    expect(checkboxes).toHaveLength(3)
    expect(
      screen.getByText('resources.user.message.selectAllLibraries'),
    ).not.toBeNull()
  })

  it('should not render master checkbox when there is only one library', () => {
    mockLibrariesResponse([{ id: '1', name: 'Library 1' }])

    render(<SelectLibraryInput onChange={mockOnChange} value={[]} />)

    const checkboxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    expect(checkboxes).toHaveLength(1)
  })

  it('should handle master checkbox selection and deselection', () => {
    const mockLibraries: LibraryRecord[] = [
      { id: '1', name: 'Library 1' },
      { id: '2', name: 'Library 2' },
    ]
    mockLibrariesResponse(mockLibraries)

    render(<SelectLibraryInput onChange={mockOnChange} value={[]} />)

    const checkboxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    const masterCheckbox = checkboxes[0]

    fireEvent.click(masterCheckbox)
    expect(mockOnChange).toHaveBeenCalledWith(['1', '2'])

    cleanup()
    mockOnChange.mockClear()

    render(
      <SelectLibraryInput
        onChange={mockOnChange}
        value={['1', '2'] as string[]}
      />,
    )
    const checkboxes2 = screen.getAllByRole('checkbox') as HTMLInputElement[]
    const masterCheckbox2 = checkboxes2[0]

    fireEvent.click(masterCheckbox2)
    expect(mockOnChange).toHaveBeenCalledWith([])
  })

  it('should show master checkbox as indeterminate when some libraries are selected', () => {
    const mockLibraries: LibraryRecord[] = [
      { id: '1', name: 'Library 1' },
      { id: '2', name: 'Library 2' },
    ]
    mockLibrariesResponse(mockLibraries)

    render(
      <SelectLibraryInput onChange={mockOnChange} value={['1'] as string[]} />,
    )

    const checkboxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    const masterCheckbox = checkboxes[0]

    expect(masterCheckbox.checked).toBe(false)
  })

  describe('New User Default Library Selection', () => {
    type LibraryConfig = {
      id: string
      name: string
      defaultNewUsers?: boolean
    }

    const createMockLibraries = (libraryConfigs: LibraryConfig[]) =>
      libraryConfigs.map(({ id, name, defaultNewUsers }) => ({
        id,
        name,
        ...(defaultNewUsers !== undefined && { defaultNewUsers }),
      }))

    const setupMockLibraries = (
      libraryConfigs: LibraryConfig[],
      isLoading = false,
    ) => {
      const libraries = createMockLibraries(libraryConfigs)
      mockLibrariesResponse(libraries, isLoading)
      return { libraries }
    }

    beforeEach(() => {
      mockOnChange.mockClear()
    })

    it('should pre-select default libraries for new users', () => {
      setupMockLibraries([
        { id: '1', name: 'Library 1', defaultNewUsers: true },
        { id: '2', name: 'Library 2', defaultNewUsers: false },
        { id: '3', name: 'Library 3', defaultNewUsers: true },
      ])

      render(
        <SelectLibraryInput
          onChange={mockOnChange}
          value={[]}
          isNewUser={true}
        />,
      )

      expect(mockOnChange).toHaveBeenCalledWith(['1', '3'])
    })

    it('should not pre-select default libraries if new user already has values', () => {
      setupMockLibraries([
        { id: '1', name: 'Library 1', defaultNewUsers: true },
        { id: '2', name: 'Library 2', defaultNewUsers: false },
      ])

      render(
        <SelectLibraryInput
          onChange={mockOnChange}
          value={['2'] as string[]}
          isNewUser={true}
        />,
      )

      expect(mockOnChange).not.toHaveBeenCalled()
    })

    it('should not pre-select libraries while data is still loading', () => {
      setupMockLibraries(
        [{ id: '1', name: 'Library 1', defaultNewUsers: true }],
        true,
      )

      render(
        <SelectLibraryInput
          onChange={mockOnChange}
          value={[]}
          isNewUser={true}
        />,
      )

      expect(mockOnChange).not.toHaveBeenCalled()
    })

    it('should not pre-select anything if no libraries have defaultNewUsers flag', () => {
      setupMockLibraries([
        { id: '1', name: 'Library 1', defaultNewUsers: false },
        { id: '2', name: 'Library 2', defaultNewUsers: false },
      ])

      render(
        <SelectLibraryInput
          onChange={mockOnChange}
          value={[]}
          isNewUser={true}
        />,
      )

      expect(mockOnChange).not.toHaveBeenCalled()
    })

    it('should reset initialization state when isNewUser prop changes', () => {
      setupMockLibraries([
        { id: '1', name: 'Library 1', defaultNewUsers: true },
      ])

      const { rerender } = render(
        <SelectLibraryInput
          onChange={mockOnChange}
          value={[]}
          isNewUser={false}
        />,
      )

      expect(mockOnChange).not.toHaveBeenCalled()

      rerender(
        <SelectLibraryInput
          onChange={mockOnChange}
          value={[]}
          isNewUser={true}
        />,
      )

      expect(mockOnChange).toHaveBeenCalledWith(['1'])
    })

    it('should not override pre-selection when value prop is empty for new users', () => {
      setupMockLibraries([
        { id: '1', name: 'Library 1', defaultNewUsers: true },
        { id: '2', name: 'Library 2', defaultNewUsers: false },
      ])

      const { rerender } = render(
        <SelectLibraryInput
          onChange={mockOnChange}
          value={[]}
          isNewUser={true}
        />,
      )

      expect(mockOnChange).toHaveBeenCalledWith(['1'])
      mockOnChange.mockClear()

      rerender(
        <SelectLibraryInput
          onChange={mockOnChange}
          value={[]}
          isNewUser={true}
        />,
      )

      expect(mockOnChange).not.toHaveBeenCalled()
    })

    it('should sync from value prop for existing users even when empty', () => {
      setupMockLibraries([
        { id: '1', name: 'Library 1', defaultNewUsers: true },
      ])

      render(
        <SelectLibraryInput
          onChange={mockOnChange}
          value={[]}
          isNewUser={false}
        />,
      )

      const checkboxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
      expect(checkboxes[0].checked).toBe(false)
    })

    it('should handle libraries with missing defaultNewUsers property', () => {
      setupMockLibraries([
        { id: '1', name: 'Library 1', defaultNewUsers: true },
        { id: '2', name: 'Library 2' },
        { id: '3', name: 'Library 3', defaultNewUsers: false },
      ])

      render(
        <SelectLibraryInput
          onChange={mockOnChange}
          value={[]}
          isNewUser={true}
        />,
      )

      expect(mockOnChange).toHaveBeenCalledWith(['1'])
    })
  })
})
