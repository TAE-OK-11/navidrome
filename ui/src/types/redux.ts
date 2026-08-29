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
  refresh?: {
    lastReceived: number
    resources: unknown
  }
}

export type SettingsState = {
  notifications: boolean
  toggleableFields: Record<string, unknown>
  omittedFields: Record<string, unknown>
  sidebarPlaylistsOnlyFavourites: boolean
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
  onSuccess?: () => void
  duplicateIds?: string[]
}

export type DownloadMenuDialogState = {
  open: boolean
  record?: unknown
  recordType?: string
}

export type ExpandInfoDialogState = {
  open: boolean
  record?: unknown
}

export type ListenBrainzTokenDialogState = {
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
