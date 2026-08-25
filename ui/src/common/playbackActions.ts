import subsonic from '../subsonic/index'
import { playTracks } from '../actions/index'

type PlaybackId = string | number

type ReplayGain = {
  albumGain?: number
  albumPeak?: number
  trackGain?: number
  trackPeak?: number
}

export type PlaybackSong = {
  id: PlaybackId
  mediaFileId?: PlaybackId
  replayGain?: ReplayGain
  [key: string]: unknown
}

type PlaybackOptions = {
  seedRecord?: PlaybackSong | null
  shuffle?: boolean
}

type PlaybackDispatch = (action: ReturnType<typeof playTracks>) => unknown
type Notify = (
  message: string,
  options?: { type?: 'warning' | 'info' },
) => unknown

const shuffleArray = <T>(array: readonly T[]): T[] => {
  const shuffled = [...array]
  for (let i = shuffled.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[shuffled[i], shuffled[j]] = [shuffled[j], shuffled[i]]
  }
  return shuffled
}

const mapReplayGain = (song: PlaybackSong): PlaybackSong => {
  const { replayGain: rg } = song
  if (!rg) {
    return song
  }

  return {
    ...song,
    ...(rg.albumGain !== undefined && { rgAlbumGain: rg.albumGain }),
    ...(rg.albumPeak !== undefined && { rgAlbumPeak: rg.albumPeak }),
    ...(rg.trackGain !== undefined && { rgTrackGain: rg.trackGain }),
    ...(rg.trackPeak !== undefined && { rgTrackPeak: rg.trackPeak }),
  }
}

export const processSongsForPlayback = (songs: PlaybackSong[]) => {
  const songData: Record<PlaybackId, PlaybackSong> = {}
  const ids: PlaybackId[] = []
  songs.forEach((s) => {
    const song = mapReplayGain(s)
    songData[song.id] = song
    ids.push(song.id)
  })
  return { songData, ids }
}

export const playSimilar = async (
  dispatch: PlaybackDispatch,
  notify: Notify,
  id: string,
  options: PlaybackOptions = {},
): Promise<void> => {
  const { seedRecord = null, shuffle = false } = options

  const res = await subsonic.getSimilarSongs2(id, 100)
  const data = res.json['subsonic-response']

  if (data.status !== 'ok') {
    throw new Error(
      `Error fetching similar songs: ${data.error?.message || 'Unknown error'} (Code: ${data.error?.code || 'unknown'})`,
    )
  }

  let songs: PlaybackSong[] = data.similarSongs2?.song || []

  // Randomize similar songs if requested
  if (shuffle) {
    songs = shuffleArray(songs)
  }

  // If no similar songs found and no seed, show warning
  if (!songs.length && !seedRecord) {
    notify('message.noSimilarSongsFound', { type: 'warning' })
    return
  }

  const { songData, ids } = processSongsForPlayback(songs)

  // Prepend seed song if provided
  if (seedRecord) {
    const seedId = seedRecord.mediaFileId || seedRecord.id
    // Remove seed from similar songs if it appears there
    const filteredIds = ids.filter((songId) => songId !== seedId)
    songData[seedId] = mapReplayGain(seedRecord)
    dispatch(playTracks(songData, [seedId, ...filteredIds]))
  } else {
    dispatch(playTracks(songData, ids))
  }
}
