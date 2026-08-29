import {
  ADD_TO_PLAYLIST_CLOSE,
  ADD_TO_PLAYLIST_OPEN,
  DOWNLOAD_MENU_ALBUM,
  DOWNLOAD_MENU_ARTIST,
  DOWNLOAD_MENU_CLOSE,
  DOWNLOAD_MENU_OPEN,
  DOWNLOAD_MENU_PLAY,
  DOWNLOAD_MENU_SONG,
  DUPLICATE_SONG_WARNING_OPEN,
  DUPLICATE_SONG_WARNING_CLOSE,
  EXTENDED_INFO_OPEN,
  EXTENDED_INFO_CLOSE,
  LISTENBRAINZ_TOKEN_OPEN,
  LISTENBRAINZ_TOKEN_CLOSE,
  SAVE_QUEUE_OPEN,
  SAVE_QUEUE_CLOSE,
  SHARE_MENU_OPEN,
  SHARE_MENU_CLOSE,
} from '../actions'
import type {
  AddToPlaylistDialogState,
  DownloadMenuDialogState,
  ExpandInfoDialogState,
  ListenBrainzTokenDialogState,
  SaveQueueDialogState,
  ShareDialogState,
  UnknownAction,
} from '../types/redux'

export const shareDialogReducer = (
  previousState: ShareDialogState = {
    open: false,
    ids: [],
    resource: '',
    name: '',
  },
  payload: UnknownAction,
): ShareDialogState => {
  const { type, ids, resource, name, label } = payload
  switch (type) {
    case SHARE_MENU_OPEN:
      return {
        ...previousState,
        open: true,
        ids: (ids ?? []) as string[],
        resource: typeof resource === 'string' ? resource : '',
        name: typeof name === 'string' ? name : '',
        label: typeof label === 'string' ? label : undefined,
      }
    case SHARE_MENU_CLOSE:
      return {
        ...previousState,
        open: false,
      }
    default:
      return previousState
  }
}

export const addToPlaylistDialogReducer = (
  previousState: AddToPlaylistDialogState = {
    open: false,
    duplicateSong: false,
  },
  payload: UnknownAction,
): AddToPlaylistDialogState => {
  const { type } = payload
  switch (type) {
    case ADD_TO_PLAYLIST_OPEN:
      return {
        ...previousState,
        open: true,
        selectedIds: (payload.selectedIds ?? []) as string[],
        onSuccess:
          typeof payload.onSuccess === 'function'
            ? (payload.onSuccess as () => void)
            : undefined,
      }
    case ADD_TO_PLAYLIST_CLOSE:
      return { ...previousState, open: false, onSuccess: undefined }
    case DUPLICATE_SONG_WARNING_OPEN:
      return {
        ...previousState,
        duplicateSong: true,
        duplicateIds: (payload.duplicateIds ?? []) as string[],
      }
    case DUPLICATE_SONG_WARNING_CLOSE:
      return { ...previousState, duplicateSong: false }
    default:
      return previousState
  }
}

export const downloadMenuDialogReducer = (
  previousState: DownloadMenuDialogState = {
    open: false,
  },
  payload: UnknownAction,
): DownloadMenuDialogState => {
  const { type } = payload
  switch (type) {
    case DOWNLOAD_MENU_OPEN: {
      switch (payload.recordType) {
        case DOWNLOAD_MENU_ALBUM:
        case DOWNLOAD_MENU_ARTIST:
        case DOWNLOAD_MENU_PLAY:
        case DOWNLOAD_MENU_SONG: {
          return {
            ...previousState,
            open: true,
            record: payload.record,
            recordType: String(payload.recordType),
          }
        }
        default: {
          return {
            ...previousState,
            open: true,
            record: payload.record,
            recordType: undefined,
          }
        }
      }
    }
    case DOWNLOAD_MENU_CLOSE: {
      return {
        ...previousState,
        open: false,
        recordType: undefined,
      }
    }
    default:
      return previousState
  }
}

export const expandInfoDialogReducer = (
  previousState: ExpandInfoDialogState = {
    open: false,
    record: undefined,
  },
  payload: UnknownAction,
): ExpandInfoDialogState => {
  const { type } = payload
  switch (type) {
    case EXTENDED_INFO_OPEN:
      return {
        ...previousState,
        open: true,
        record: payload.record,
      }
    case EXTENDED_INFO_CLOSE:
      return {
        ...previousState,
        open: false,
        record: undefined,
      }
    default:
      return previousState
  }
}

export const listenBrainzTokenDialogReducer = (
  previousState: ListenBrainzTokenDialogState = {
    open: false,
  },
  payload: UnknownAction,
): ListenBrainzTokenDialogState => {
  const { type } = payload
  switch (type) {
    case LISTENBRAINZ_TOKEN_OPEN:
      return {
        ...previousState,
        open: true,
      }
    case LISTENBRAINZ_TOKEN_CLOSE:
      return {
        ...previousState,
        open: false,
      }
    default:
      return previousState
  }
}

export const saveQueueDialogReducer = (
  previousState: SaveQueueDialogState = { open: false },
  payload: UnknownAction,
): SaveQueueDialogState => {
  const { type } = payload
  switch (type) {
    case SAVE_QUEUE_OPEN:
      return { ...previousState, open: true }
    case SAVE_QUEUE_CLOSE:
      return { ...previousState, open: false }
    default:
      return previousState
  }
}
