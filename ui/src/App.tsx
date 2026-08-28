// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { useMemo } from 'react'
import ReactGA from 'react-ga4'
import { Provider } from 'react-redux'
import { Admin as RAAdmin, Resource } from 'react-admin'
import { createTheme } from '@mui/material/styles'
import { HotKeys } from 'react-hotkeys'
import dataProvider from './dataProvider'
import authProvider from './authProvider'
import { Layout, Login } from './layout'
import transcoding from './transcoding'
import player from './player'
import user from './user'
import song from './song'
import album from './album'
import artist from './artist'
import playlist from './playlist'
import radio from './radio'
import share from './share'
import library from './library'
import plugin from './plugin'
import { Player } from './audioplayer'
import AppRoutes from './routes'
import {
  libraryReducer,
  themeReducer,
  addToPlaylistDialogReducer,
  expandInfoDialogReducer,
  listenBrainzTokenDialogReducer,
  saveQueueDialogReducer,
  playerReducer,
  albumViewReducer,
  activityReducer,
  settingsReducer,
  replayGainReducer,
  downloadMenuDialogReducer,
  shareDialogReducer,
  transcodingReducer,
} from './reducers'
import createAdminStore from './store/createAdminStore'
import { i18nProvider } from './i18n'
import config, { shareInfo } from './config'
import { keyMap } from './hotkeys'
import useChangeThemeColor from './useChangeThemeColor'
import useCurrentTheme from './themes/useCurrentTheme'
import SharePlayer from './share/SharePlayer'
import { HTML5Backend } from 'react-dnd-html5-backend'
import { DndProvider } from 'react-dnd'
import missing from './missing/index'
import ClientError from './layout/ClientError'
import modernizeTheme from './themes/modernizeTheme'

if (config.gaTrackingId) {
  ReactGA.initialize(config.gaTrackingId)
  const trackPage = () =>
    ReactGA.send({ hitType: 'pageview', page: window.location.hash || '/' })
  trackPage()
  window.addEventListener('hashchange', trackPage)
}

const adminStore = createAdminStore({
  customReducers: {
    library: libraryReducer,
    player: playerReducer,
    albumView: albumViewReducer,
    theme: themeReducer,
    addToPlaylistDialog: addToPlaylistDialogReducer,
    downloadMenuDialog: downloadMenuDialogReducer,
    expandInfoDialog: expandInfoDialogReducer,
    listenBrainzTokenDialog: listenBrainzTokenDialogReducer,
    saveQueueDialog: saveQueueDialogReducer,
    shareDialog: shareDialogReducer,
    activity: activityReducer,
    settings: settingsReducer,
    replayGain: replayGainReducer,
    transcoding: transcodingReducer,
  },
})

const App = () => (
  <Provider store={adminStore}>
    <Admin />
  </Provider>
)

const Admin = (props) => {
  const themeOptions = useCurrentTheme()
  const theme = useMemo(
    () => createTheme(modernizeTheme(themeOptions)),
    [themeOptions],
  )
  useChangeThemeColor()

  return (
    <RAAdmin
      disableTelemetry
      requireAuth
      dataProvider={dataProvider}
      authProvider={authProvider}
      i18nProvider={i18nProvider}
      layout={Layout}
      loginPage={Login}
      error={ClientError}
      theme={theme}
      {...props}
    >
      {(permissions) => (
        <>
          <Resource
            name="album"
            {...album}
            options={{ subMenu: 'albumList' }}
          />
          <Resource name="artist" {...artist} />
          <Resource name="song" {...song} />
          <Resource
            name="radio"
            {...(permissions === 'admin' ? radio.admin : radio.all)}
          />
          {config.enableSharing && <Resource name="share" {...share} />}
          <Resource
            name="playlist"
            {...playlist}
            options={{ subMenu: 'playlist' }}
          />
          <Resource name="user" {...user} options={{ subMenu: 'settings' }} />
          <Resource
            name="player"
            {...player}
            options={{ subMenu: 'settings' }}
          />
          {permissions === 'admin' ? (
            <Resource
              name="transcoding"
              {...transcoding}
              options={{ subMenu: 'settings' }}
            />
          ) : (
            <Resource name="transcoding" />
          )}
          {permissions === 'admin' && (
            <Resource
              name="library"
              {...library}
              options={{ subMenu: 'settings' }}
            />
          )}
          {permissions === 'admin' && (
            <Resource
              name="missing"
              {...missing}
              options={{ subMenu: 'settings' }}
            />
          )}
          {permissions === 'admin' && config.pluginsEnabled && (
            <Resource
              name="plugin"
              {...plugin}
              options={{ subMenu: 'settings' }}
            />
          )}
          <Resource name="translation" />
          <Resource name="genre" />
          <Resource name="tag" />
          <Resource name="playlistTrack" />
          <Resource name="keepalive" />
          <Resource name="insights" />
          <Resource name="config" />
          {AppRoutes()}
          <Player />
        </>
      )}
    </RAAdmin>
  )
}

const AppWithHotkeys = () => {
  const language = localStorage.getItem('locale') || 'en'
  document.documentElement.lang = language
  if (config.enableSharing && shareInfo) {
    return <SharePlayer />
  }
  return (
    <HotKeys keyMap={keyMap}>
      <DndProvider backend={HTML5Backend}>
        <App />
      </DndProvider>
    </HotKeys>
  )
}

export default AppWithHotkeys
