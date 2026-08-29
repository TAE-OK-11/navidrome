import React from 'react'
import { render, fireEvent, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  createTheme,
  ThemeProvider,
  StyledEngineProvider,
} from '@mui/material/styles'
import {
  AdminContext,
  ListContextProvider,
  ResourceContextProvider,
  TextField,
} from 'react-admin'
import { DiscSubtitleRow, SongDatagrid } from './SongDatagrid'

vi.mock('../subsonic', () => ({
  default: { getDiscCoverArtUrl: () => 'http://localhost/cover.jpg' },
}))

vi.mock('react-redux', () => ({ useDispatch: () => vi.fn() }))

vi.mock('../common', () => ({ AlbumContextMenu: () => null }))

vi.mock('react-dnd', () => ({ useDrag: () => [{}, vi.fn()] }))

const record = {
  id: 'song-1',
  albumId: 'album-1',
  album: 'The Album',
  discNumber: 2,
  discSubtitle: 'Bonus Disc',
  updatedAt: '2024-01-01',
}

const renderRow = (onClick) =>
  render(
    <StyledEngineProvider injectFirst>
      <ThemeProvider theme={createTheme()}>
        <table>
          <tbody>
            <DiscSubtitleRow record={record as any} onClick={onClick} colSpan={3} />
          </tbody>
        </table>
      </ThemeProvider>
    </StyledEngineProvider>,
  )

const openLightbox = () => {
  fireEvent.click(document.querySelector('img')!)
  expect(screen.getByRole('button', { name: 'Close' })).toBeTruthy()
}

describe('DiscSubtitleRow', () => {
  beforeEach(() => vi.clearAllMocks())

  it('plays the disc when the row is clicked', () => {
    const onClick = vi.fn()
    renderRow(onClick)
    fireEvent.click(screen.getByText('Bonus Disc'))
    expect(onClick).toHaveBeenCalledWith(2)
  })

  it('does not play the disc when opening the lightbox', () => {
    const onClick = vi.fn()
    renderRow(onClick)
    openLightbox()
    expect(onClick).not.toHaveBeenCalled()
  })

  it('does not play the disc when closing the lightbox', () => {
    const onClick = vi.fn()
    renderRow(onClick)
    openLightbox()
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClick).not.toHaveBeenCalled()
  })

  it('does not play the disc when clicking the lightbox backdrop', () => {
    const onClick = vi.fn()
    renderRow(onClick)
    openLightbox()
    fireEvent.click(document.querySelector('.yarl__container')!)
    expect(onClick).not.toHaveBeenCalled()
  })
})

describe('SongDatagrid', () => {
  it('renders and plays records supplied through the React-admin 5 record context', () => {
    const rowClick = vi.fn()
    const listContext = {
      data: [
        {
          id: 'song-1',
          title: 'Visible Track',
          albumId: 'album-1',
          discNumber: 1,
        },
      ],
      total: 1,
      isPending: false,
      resource: 'song',
      sort: { field: 'title', order: 'ASC' },
      selectedIds: [],
      onSelect: vi.fn(),
      onToggleItem: vi.fn(),
      setSort: vi.fn(),
    }

    render(
      <AdminContext>
        <ResourceContextProvider value="song">
          <ListContextProvider value={listContext as any}>
            <SongDatagrid bulkActionButtons={false} rowClick={rowClick}>
              <TextField source="title" />
            </SongDatagrid>
          </ListContextProvider>
        </ResourceContextProvider>
      </AdminContext>,
    )

    expect(screen.getByText('Visible Track')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Visible Track'))
    expect(rowClick).toHaveBeenCalledWith(
      'song-1',
      'song',
      expect.objectContaining({ title: 'Visible Track' }),
    )
  })
})
