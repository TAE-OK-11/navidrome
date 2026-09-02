// These defaults are only used in development mode. When bundled in the app,
// the __APP_CONFIG__ object is dynamically filled by the ServeIndex function,
// in the /server/app/serve_index.go
const defaultConfig = {
  version: 'dev',
  firstTime: false,
  baseURL: '',
  variousArtistsId: '63sqASlAfjbGMuLP4JhnZU', // See consts.VariousArtistsID in consts.go
  // Login backgrounds from https://unsplash.com/collections/1065384/music-wallpapers
  loginBackgroundURL: 'https://source.unsplash.com/collection/1065384/1600x900',
  maxSidebarPlaylists: 100,
  enableTranscodingConfig: true,
  enableDownloads: true,
  enableFavourites: true,
  losslessFormats: 'FLAC,WAV,ALAC,DSF',
  welcomeMessage: '',
  gaTrackingId: '',
  devActivityPanel: true,
  enableStarRating: true,
  defaultTheme: 'Dark',
  defaultLanguage: '',
  defaultUIVolume: 100,
  uiSearchDebounceMs: 200,
  uiCoverArtSize: 600,
  enableUserEditing: true,
  enableArtworkUpload: true,
  enableSharing: true,
  shareURL: '',
  defaultDownloadableShare: true,
  devSidebarPlaylists: true,
  lastFMEnabled: true,
  listenBrainzEnabled: true,
  libreFMEnabled: true,
  enableExternalServices: true,
  enableCoverAnimation: true,
  enableNowPlaying: true,
  playbackReportIntervalMs: 60000,
  devShowArtistPage: true,
  devUIShowConfig: true,
  devNewEventStream: false,
  enableReplayGain: true,
  defaultDownsamplingFormat: 'opus',
  publicBaseUrl: '/share',
  separator: '/',
  enableInspect: true,
  pluginsEnabled: true,
}

type AuthenticationInfo = {
  token?: string
  id: string
  name: string
  username: string
  avatar?: string
  isAdmin: boolean
  subsonicSalt: string
  subsonicToken: string
}

export type AppConfig = typeof defaultConfig & {
  auth?: AuthenticationInfo
  extAuthLogoutURL?: string
}

export type ShareInfo = {
  id: string
  downloadable: boolean
  tracks: Array<{
    id: string
    title: string
    artist: string
    duration: number
  }>
}

declare global {
  interface Window {
    __APP_CONFIG__?: string
    __SHARE_INFO__?: string
  }
}

let config: AppConfig

try {
  const appConfig = JSON.parse(
    window.__APP_CONFIG__ ?? '{}',
  ) as Partial<AppConfig>
  config = {
    ...defaultConfig,
    ...appConfig,
  }
} catch {
  config = { ...defaultConfig }
}

export let shareInfo: ShareInfo | null

try {
  shareInfo = JSON.parse(window.__SHARE_INFO__ ?? 'null') as ShareInfo | null
} catch {
  shareInfo = null
}

export default config
