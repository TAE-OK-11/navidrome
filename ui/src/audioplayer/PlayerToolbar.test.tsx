import React from 'react'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import { useMediaQuery } from '@mui/material'
import { useGetOne } from 'react-admin'
import { useDispatch } from 'react-redux'
import { useToggleLove } from '../common'
import { openSaveQueueDialog } from '../actions'
import PlayerToolbar from './PlayerToolbar'

// Mock dependencies
vi.mock('@mui/material', async () => {
  const actual = await import('@mui/material')
  return {
    ...actual,
    useMediaQuery: vi.fn(),
  }
})

vi.mock('react-admin', () => ({
  useGetOne: vi.fn(),
}))

vi.mock('react-redux', () => ({
  useDispatch: vi.fn(),
}))

vi.mock('../common', () => ({
  LoveButton: ({ className, disabled }) => (
    <button data-testid="love-button" className={className} disabled={disabled}>
      Love
    </button>
  ),
  useToggleLove: vi.fn(),
}))

vi.mock('../actions', () => ({
  openSaveQueueDialog: vi.fn(),
}))

vi.mock('../hooks/useAppHotkey', () => ({
  useAppHotkey: vi.fn(),
}))

describe('<PlayerToolbar />', () => {
  const mockToggleLove = vi.fn()
  const mockDispatch = vi.fn()
  const mockSongData = { id: 'song-1', name: 'Test Song', starred: false }

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useGetOne).mockReturnValue({ data: mockSongData, isPending: false } as any)
    vi.mocked(useToggleLove).mockReturnValue([mockToggleLove, false, false] as any)
    vi.mocked(useDispatch).mockReturnValue(mockDispatch)
    vi.mocked(openSaveQueueDialog).mockReturnValue({ type: 'OPEN_SAVE_QUEUE_DIALOG' } as any)
  })

  afterEach(cleanup)

  describe('Desktop layout', () => {
    beforeEach(() => {
      vi.mocked(useMediaQuery).mockReturnValue(true) // isDesktop = true
    })

    it('renders desktop toolbar with both buttons', () => {
      render(<PlayerToolbar id="song-1" isRadio={false} />)

      // Both buttons should be in a single list item
      const listItems = screen.getAllByRole('listitem')
      expect(listItems).toHaveLength(1)

      // Verify both buttons are rendered
      expect(screen.getByTestId('save-queue-button')).toBeInTheDocument()
      expect(screen.getByTestId('love-button')).toBeInTheDocument()

      // Verify desktop classes are applied
      expect(listItems[0].className).toContain('toolbar')
    })

    it('disables save queue button when isRadio is true', () => {
      render(<PlayerToolbar id="song-1" isRadio={true} />)

      const saveQueueButton = screen.getByTestId('save-queue-button')
      expect(saveQueueButton).toBeDisabled()
    })

    it('disables love button when conditions are met', () => {
      vi.mocked(useGetOne).mockReturnValue({ data: mockSongData, isPending: true } as any)

      render(<PlayerToolbar id="song-1" isRadio={false} />)

      const loveButton = screen.getByTestId('love-button')
      expect(loveButton).toBeDisabled()
    })

    it('opens save queue dialog when save button is clicked', () => {
      render(<PlayerToolbar id="song-1" isRadio={false} />)

      const saveQueueButton = screen.getByTestId('save-queue-button')
      fireEvent.click(saveQueueButton)

      expect(mockDispatch).toHaveBeenCalledWith({
        type: 'OPEN_SAVE_QUEUE_DIALOG',
      })
    })
  })

  describe('Mobile layout', () => {
    beforeEach(() => {
      vi.mocked(useMediaQuery).mockReturnValue(false) // isDesktop = false
    })

    it('renders mobile toolbar with buttons in separate list items', () => {
      render(<PlayerToolbar id="song-1" isRadio={false} />)

      // Each button should be in its own list item
      const listItems = screen.getAllByRole('listitem')
      expect(listItems).toHaveLength(2)

      // Verify both buttons are rendered
      expect(screen.getByTestId('save-queue-button')).toBeInTheDocument()
      expect(screen.getByTestId('love-button')).toBeInTheDocument()

      // Verify mobile classes are applied
      expect(listItems[0].className).toContain('mobileListItem')
      expect(listItems[1].className).toContain('mobileListItem')
    })

    it('disables save queue button when isRadio is true', () => {
      render(<PlayerToolbar id="song-1" isRadio={true} />)

      const saveQueueButton = screen.getByTestId('save-queue-button')
      expect(saveQueueButton).toBeDisabled()
    })

    it('disables love button when conditions are met', () => {
      vi.mocked(useGetOne).mockReturnValue({ data: mockSongData, isPending: true } as any)

      render(<PlayerToolbar id="song-1" isRadio={false} />)

      const loveButton = screen.getByTestId('love-button')
      expect(loveButton).toBeDisabled()
    })
  })

  describe('Common behavior', () => {
    it('disables buttons when id is not provided', () => {
      render(<PlayerToolbar id={undefined} isRadio={false} />)

      const loveButton = screen.getByTestId('love-button')
      expect(loveButton).toBeDisabled()
    })
  })
})
