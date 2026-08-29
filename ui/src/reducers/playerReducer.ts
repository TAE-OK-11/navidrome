import { v4 as uuidv4 } from 'uuid'
import subsonic from '../subsonic'
import { decisionService } from '../transcode'
import {
  PLAYER_ADD_TRACKS,
  PLAYER_CLEAR_QUEUE,
  PLAYER_CURRENT,
  PLAYER_PLAY_NEXT,
  PLAYER_PLAY_TRACKS,
  PLAYER_SET_TRACK,
  PLAYER_SET_VOLUME,
  PLAYER_SYNC_QUEUE,
  PLAYER_SET_MODE,
  PLAYER_REFRESH_QUEUE,
} from '../actions'
import config from '../config'
import type {
  AudioListItem,
  PlayerCurrent,
  PlayerState,
  TrackSource,
  UnknownAction,
} from '../types/redux'

const initialState: PlayerState = {
  queue: [],
  current: {},
  clear: false,
  volume: config.defaultUIVolume / 100,
  savedPlayIndex: 0,
}

const pad = (value: number): string =>
  value < 10 ? `0${value}` : String(value)

const formatSyncedLyrics = (lyrics: unknown): string => {
  if (!lyrics) return ''

  let structured: unknown
  try {
    structured = typeof lyrics === 'string' ? JSON.parse(lyrics) : lyrics
  } catch {
    return ''
  }
  if (!Array.isArray(structured)) return ''

  const output: string[] = []
  for (const entry of structured) {
    if (
      !entry ||
      typeof entry !== 'object' ||
      !('synced' in entry) ||
      !entry.synced ||
      !('line' in entry) ||
      !Array.isArray(entry.line)
    ) {
      continue
    }
    for (const line of entry.line) {
      if (!line || typeof line !== 'object' || !('start' in line)) continue
      const start = Number(line.start)
      if (!Number.isFinite(start)) continue

      let time = Math.floor(start / 10)
      const ms = time % 100
      time = Math.floor(time / 100)
      const sec = time % 60
      const min = Math.floor(time / 60) % 60
      const value =
        'value' in line && line.value != null ? String(line.value) : ''
      output.push(`[${pad(min)}:${pad(sec)}.${pad(ms)}] ${value}`)
    }
  }
  return output.length > 0 ? `${output.join('\n')}\n` : ''
}

const makeMusicSrc = (trackId: string): string | (() => Promise<string>) =>
  decisionService.getProfile()
    ? () =>
        decisionService
          .resolveStreamUrl(trackId)
          .catch(() => subsonic.streamUrl(trackId))
    : subsonic.streamUrl(trackId)

const mapToAudioLists = (item: TrackSource): AudioListItem => {
  // If item comes from a playlist, trackId is mediaFileId
  const trackId = String(item.mediaFileId ?? item.id ?? '')
  const updatedAt =
    typeof item.updatedAt === 'string' ? item.updatedAt : undefined
  const album = typeof item.album === 'string' ? item.album : undefined

  if (item.isRadio) {
    return {
      trackId,
      uuid: uuidv4(),
      name: typeof item.name === 'string' ? item.name : undefined,
      song: item,
      musicSrc: typeof item.streamUrl === 'string' ? item.streamUrl : undefined,
      cover: typeof item.cover === 'string' ? item.cover : undefined,
      isRadio: true,
    }
  }

  const lyricText = formatSyncedLyrics(item.lyrics)

  return {
    trackId,
    uuid: uuidv4(),
    song: item,
    name: typeof item.title === 'string' ? item.title : undefined,
    lyric: lyricText,
    singer: typeof item.artist === 'string' ? item.artist : undefined,
    duration: typeof item.duration === 'number' ? item.duration : undefined,
    musicSrc: makeMusicSrc(trackId),
    cover: subsonic.getCoverArtUrl(
      {
        id: trackId,
        updatedAt,
        album,
      },
      300,
    ),
  }
}

const reduceClearQueue = (): PlayerState => ({ ...initialState, clear: true })

const reducePlayTracks = (
  state: PlayerState,
  { data, id }: UnknownAction,
): PlayerState => {
  let playIndex = 0
  const tracks = (data ?? {}) as Record<string, TrackSource>
  const queue = Object.keys(tracks).map((key, idx) => {
    if (key === id) {
      playIndex = idx
    }
    return mapToAudioLists(tracks[key])
  })
  return {
    ...state,
    queue,
    playIndex,
    clear: true,
  }
}

const reduceSetTrack = (
  state: PlayerState,
  { data }: UnknownAction,
): PlayerState => {
  return {
    ...state,
    queue: [mapToAudioLists((data ?? {}) as TrackSource)],
    playIndex: 0,
    clear: true,
  }
}

const reduceAddTracks = (
  state: PlayerState,
  { data }: UnknownAction,
): PlayerState => {
  const queue = state.queue.slice()
  const tracks = (data ?? {}) as Record<string, TrackSource>
  Object.keys(tracks).forEach((id) => {
    queue.push(mapToAudioLists(tracks[id]))
  })
  return { ...state, queue, clear: false }
}

const reducePlayNext = (
  state: PlayerState,
  { data }: UnknownAction,
): PlayerState => {
  const tracks = (data ?? {}) as Record<string, TrackSource>
  const newTracks = Object.keys(tracks).map((id) => mapToAudioLists(tracks[id]))
  const newQueue: AudioListItem[] = []
  const current = state.current || {}
  let foundPos = false
  state.queue.forEach((item) => {
    newQueue.push(item)
    if (item.uuid === current.uuid) {
      foundPos = true
      newQueue.push(...newTracks)
    }
  })
  if (!foundPos) {
    newQueue.push(...newTracks)
  }

  return {
    ...state,
    queue: newQueue,
    clear: true,
  }
}

const reduceSetVolume = (
  state: PlayerState,
  { data }: UnknownAction,
): PlayerState => {
  const payload = (data ?? {}) as { volume?: number }
  return {
    ...state,
    volume: payload.volume ?? state.volume,
  }
}

const reduceSyncQueue = (
  state: PlayerState,
  { data }: UnknownAction,
): PlayerState => {
  const payload = (data ?? {}) as {
    audioInfo?: PlayerCurrent
    audioLists?: AudioListItem[]
  }
  const audioLists = payload.audioLists ?? []
  // Keep clear and playIndex alive when there is a pending track switch.
  // A switch is pending when playIndex is set AND either:
  //   - playIndex differs from savedPlayIndex, OR
  //   - clear is true (a new queue was loaded, e.g. after clearQueue + playTracks)
  // The clear check handles the edge case where both playIndex and
  // savedPlayIndex are 0 (close player then play a new album from track 1).
  const hasPendingSwitch =
    state.playIndex != null &&
    (state.clear || state.playIndex !== state.savedPlayIndex)
  return {
    ...state,
    queue: audioLists,
    clear: hasPendingSwitch ? state.clear : false,
    playIndex: hasPendingSwitch ? state.playIndex : undefined,
  }
}

const reduceCurrent = (
  state: PlayerState,
  { data }: UnknownAction,
): PlayerState => {
  const payload = (data ?? {}) as PlayerCurrent & { ended?: boolean }
  const current = payload.ended ? {} : payload
  const savedPlayIndex = state.queue.findIndex(
    (item) => item.uuid === current.uuid,
  )
  // When a track selection is pending (playIndex is set), keep it alive
  // until the music player confirms it actually switched to the requested
  // track. Without this, a premature onAudioPlay callback for the
  // still-playing old track would overwrite the pending selection.
  const pending = state.playIndex != null && savedPlayIndex !== state.playIndex
  return {
    ...state,
    current,
    playIndex: pending ? state.playIndex : undefined,
    clear: pending ? state.clear : false,
    savedPlayIndex: pending ? state.savedPlayIndex : savedPlayIndex,
    volume: typeof payload.volume === 'number' ? payload.volume : state.volume,
  }
}

const reduceMode = (
  state: PlayerState,
  { data }: UnknownAction,
): PlayerState => {
  const payload = (data ?? {}) as { mode?: string }
  return {
    ...state,
    mode: payload.mode,
  }
}

export const playerReducer = (
  previousState: PlayerState = initialState,
  payload: UnknownAction,
): PlayerState => {
  const { type } = payload
  switch (type) {
    case PLAYER_CLEAR_QUEUE:
      return reduceClearQueue()
    case PLAYER_PLAY_TRACKS:
      return reducePlayTracks(previousState, payload)
    case PLAYER_SET_TRACK:
      return reduceSetTrack(previousState, payload)
    case PLAYER_ADD_TRACKS:
      return reduceAddTracks(previousState, payload)
    case PLAYER_PLAY_NEXT:
      return reducePlayNext(previousState, payload)
    case PLAYER_SET_VOLUME:
      return reduceSetVolume(previousState, payload)
    case PLAYER_SYNC_QUEUE:
      return reduceSyncQueue(previousState, payload)
    case PLAYER_CURRENT:
      return reduceCurrent(previousState, payload)
    case PLAYER_SET_MODE:
      return reduceMode(previousState, payload)
    case PLAYER_REFRESH_QUEUE: {
      const resolvedUrls = (payload.data ?? {}) as Record<string, string>
      return {
        ...previousState,
        queue: previousState.queue.map((item) => ({
          ...item,
          musicSrc: item.isRadio
            ? item.musicSrc
            : resolvedUrls[item.trackId] || subsonic.streamUrl(item.trackId),
        })),
        clear: true,
        autoPlay: false,
        playIndex:
          previousState.savedPlayIndex >= 0 ? previousState.savedPlayIndex : 0,
      }
    }
    default:
      return previousState
  }
}
