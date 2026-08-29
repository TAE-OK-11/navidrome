import React from 'react'
import { render, screen, fireEvent, act } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { Provider } from 'react-redux'
import { createStore, combineReducers } from 'redux'
import {
  ThemeProvider,
  StyledEngineProvider,
  createTheme,
} from '@mui/material/styles'
import { settingsReducer, activityReducer } from '../reducers'
import { processEvent, EVENT_REFRESH_RESOURCE } from '../actions'
import PlaylistsSubMenu from './PlaylistsSubMenu'

const mockUseGetList = vi.fn()

vi.mock('../config', () => ({
  // losslessFormats is read at module-load time by common/QualityInfo.jsx,
  // pulled in transitively via the '../common' barrel file
  default: {
    enableFavourites: true,
    maxSidebarPlaylists: 100,
    losslessFormats: '',
  },
}))

vi.mock('react-dnd', () => ({
  useDrop: () => [{}, () => {}],
}))

vi.mock('react-router-dom', () => ({ useNavigate: () => vi.fn() }))

vi.mock('react-admin', async (importOriginal) => {
  const actual = (await importOriginal()) as Record<string, unknown>
  return {
    ...actual,
    useTranslate: () => (x) => x,
    useDataProvider: () => ({ addToPlaylist: vi.fn() }),
    useNotify: () => vi.fn(),
    useGetList: (resource, params) => mockUseGetList(resource, params),
    MenuItemLink: ({ primaryText }) => <div>{primaryText}</div>,
  }
})

const playlists = {
  'pl-1': { id: 'pl-1', name: 'Mine', ownerId: 'user-1' },
  'pl-2': { id: 'pl-2', name: 'Theirs', ownerId: 'user-2' },
}

const renderMenu = (preloadedSettings = {}) => {
  const store = createStore(
    combineReducers({
      settings: settingsReducer as any,
      activity: activityReducer as any,
    }),
    {
      settings: preloadedSettings,
      activity: {},
    },
  )
  const theme = createTheme()
  render(
    <Provider store={store}>
      <StyledEngineProvider injectFirst>
        <ThemeProvider theme={theme}>
          <PlaylistsSubMenu
            state={{ menuPlaylists: true, menuSharedPlaylists: true }}
            setState={vi.fn()}
            sidebarIsOpen={true}
            dense={false}
          />
        </ThemeProvider>
      </StyledEngineProvider>
    </Provider>,
  )
  return store
}

const lastQuery = () =>
  mockUseGetList.mock.calls[mockUseGetList.mock.calls.length - 1][1]

describe('<PlaylistsSubMenu />', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.setItem('userId', 'user-1')
    mockUseGetList.mockReturnValue({
      data: Object.values(playlists),
      isPending: false,
    })
    // SubMenu uses MUI's useMediaQuery, which needs window.matchMedia in jsdom
    window.matchMedia = ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    })) as typeof window.matchMedia
    // OverflowTooltip (via MenuItemLink) needs ResizeObserver, unavailable in jsdom
    window.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  })

  it('queries without a starred filter by default', () => {
    renderMenu()
    expect(lastQuery().filter).toEqual({})
    expect(screen.getByText('Mine')).not.toBeNull()
    expect(screen.getByText('Theirs')).not.toBeNull()
  })

  it('adds the starred filter when favourites-only is enabled', () => {
    renderMenu({ sidebarPlaylistsOnlyFavourites: true })
    expect(lastQuery().filter).toEqual({ starred: true })
  })

  it('toggles the setting when the heart action is clicked', () => {
    const store = renderMenu()
    fireEvent.click(screen.getByTitle('menu.onlyFavourites'))
    expect(
      (store.getState() as any).settings.sidebarPlaylistsOnlyFavourites,
    ).toBe(true)
    expect(lastQuery().filter).toEqual({ starred: true })
  })

  it('refetches on a playlist SSE event when favourites-only is on', async () => {
    const store = renderMenu({ sidebarPlaylistsOnlyFavourites: true })
    const before = lastQuery().meta.refresh
    // useRefreshOnEvents compares Date.now() timestamps; make sure it advances
    await act(() => new Promise((resolve) => setTimeout(resolve, 5)))
    act(() => {
      store.dispatch(
        processEvent(EVENT_REFRESH_RESOURCE, { playlist: ['pl-1'] }),
      )
    })
    expect(lastQuery().meta.refresh).toBe(before + 1)
  })

  it('does not change the query signature on an SSE event when favourites-only is off', async () => {
    const store = renderMenu()
    const before = JSON.stringify(lastQuery())
    await act(() => new Promise((resolve) => setTimeout(resolve, 5)))
    act(() => {
      store.dispatch(
        processEvent(EVENT_REFRESH_RESOURCE, { playlist: ['pl-1'] }),
      )
    })
    // Signature unchanged → useQueryWithStore dedupes, no wasted refetch
    expect(lastQuery().meta).toBeUndefined()
    expect(JSON.stringify(lastQuery())).toBe(before)
  })
})
