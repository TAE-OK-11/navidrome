export type UnknownAction = {
  type: string
  [key: string]: unknown
}

export type LibraryInfo = {
  id: string
  name: string
}

export type LibraryState = {
  userLibraries: LibraryInfo[]
  selectedLibraries: string[]
}

export type ThemeState = string

export type TranscodingState = {
  browserProfile: unknown | null
}

export type ScanStatus = {
  scanning: boolean
  folderCount: number
  count: number
  error: string
  elapsedTime: number
  scanType?: string
}

export type NowPlayingEntry = {
  username: string
  playerName?: string
  title?: string
  artist?: string
  album?: string
  albumId?: string
  artistId?: string
  albumArtist?: string
  albumArtistId?: string
  duration?: number
  state?: string
  positionMs?: number
  playbackRate?: number
  _fetchedAt?: number
}

export type ActivityState = {
  scanStatus: ScanStatus
  serverStart: {
    version: string
    startTime?: number
  }
  nowPlayingCount: number
  nowPlayingLastUpdate: number
  streamReconnected: number
  refresh?: ActivityRefresh
}

export type SettingsState = {
  notifications: boolean
  toggleableFields: Record<string, Record<string, boolean>>
  omittedFields: Record<string, string[]>
  sidebarPlaylistsOnlyFavourites: boolean
}

export type ActivityRefresh = {
  lastReceived: number
  resources: Record<string, string[] | string>
}

export type TrackSource = Record<string, unknown>

export type AudioListItem = {
  trackId: string
  uuid: string
  name?: string
  song?: TrackSource
  musicSrc?: string | (() => Promise<string>)
  cover?: string
  isRadio?: boolean
  lyric?: string
  singer?: string
  duration?: number
}

export type PlayerCurrent = {
  uuid?: string
  ended?: boolean
  volume?: number
  [key: string]: unknown
}

export type PlayerState = {
  queue: AudioListItem[]
  current: PlayerCurrent
  clear: boolean
  volume: number
  savedPlayIndex: number
  playIndex?: number
  mode?: string
  autoPlay?: boolean
}

export type ReplayGainState = {
  gainMode: string
  preAmp: number
}

export type AlbumViewState = {
  grid: boolean
}

export type ShareDialogState = {
  open: boolean
  ids: string[]
  resource: string
  name: string
  label?: string
}

export type AddToPlaylistDialogState = {
  open: boolean
  duplicateSong: boolean
  selectedIds?: string[]
  onSuccess?: (value?: unknown, len?: number) => void
  duplicateIds?: string[]
}

export type DownloadMenuDialogState = {
  open: boolean
  record?: {
    id?: string | number
    mediaFileId?: string | number
    name?: string
    title?: string
    size?: number
    duration?: number
  }
  recordType?: string
}

export type ExpandInfoDialogState = {
  open: boolean
  record?: Record<string, unknown>
}

export type ListenBrainzTokenDialogState = {
  open: boolean
}

export type LibreFmSessionDialogState = {
  open: boolean
}

export type SaveQueueDialogState = {
  open: boolean
}

export type NavidromeRootState = {
  library: LibraryState
  player: PlayerState
  albumView: AlbumViewState
  theme: ThemeState
  addToPlaylistDialog: AddToPlaylistDialogState
  downloadMenuDialog: DownloadMenuDialogState
  expandInfoDialog: ExpandInfoDialogState
  listenBrainzTokenDialog: ListenBrainzTokenDialogState
  libreFmSessionDialog: LibreFmSessionDialogState
  saveQueueDialog: SaveQueueDialogState
  shareDialog: ShareDialogState
  activity: ActivityState
  settings: SettingsState
  replayGain: ReplayGainState
  transcoding: TranscodingState
}

export type PersistedState = Partial<
  Pick<NavidromeRootState, 'theme' | 'library' | 'albumView' | 'settings'>
> & {
  player?: Pick<PlayerState, 'queue' | 'volume' | 'savedPlayIndex'>
}

export type AppState = {
  albumView: AlbumViewState
  activity?: ActivityState
  settings: SettingsState
  player?: {
    current?: {
      trackId?: string
      paused?: boolean
    }
  }
}
