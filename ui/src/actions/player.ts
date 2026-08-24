export const PLAYER_ADD_TRACKS = 'PLAYER_ADD_TRACKS'
export const PLAYER_PLAY_NEXT = 'PLAYER_PLAY_NEXT'
export const PLAYER_SET_TRACK = 'PLAYER_SET_TRACK'
export const PLAYER_SYNC_QUEUE = 'PLAYER_SYNC_QUEUE'
export const PLAYER_CLEAR_QUEUE = 'PLAYER_CLEAR_QUEUE'
export const PLAYER_PLAY_TRACKS = 'PLAYER_PLAY_TRACKS'
export const PLAYER_CURRENT = 'PLAYER_CURRENT'
export const PLAYER_SET_VOLUME = 'PLAYER_SET_VOLUME'
export const PLAYER_SET_MODE = 'PLAYER_SET_MODE'
export const TRANSCODING_SET_PROFILE = 'TRANSCODING_SET_PROFILE'
export const PLAYER_REFRESH_QUEUE = 'PLAYER_REFRESH_QUEUE'

export const setTrack = (data) => ({
  type: PLAYER_SET_TRACK,
  data,
})

export const filterSongs = (data, ids) => {
  const filteredData = {}
  if (ids) {
    const songsById = Array.isArray(data)
      ? Object.fromEntries(data.map((song) => [song.id, song]))
      : data || {}
    for (const id of ids) {
      const song = songsById[id]
      if (song && !song.missing) filteredData[id] = song
    }
    return filteredData
  }

  if (Array.isArray(data)) {
    for (const song of data) {
      if (!song.missing) filteredData[song.id] = song
    }
  } else {
    const entries =
      data && typeof data === 'object'
        ? Object.entries(data as Record<string, { missing?: boolean }>)
        : []
    for (const [id, song] of entries) {
      // Preserve caller-provided keys such as the shuffled queue's `_id` keys.
      if (!song.missing) filteredData[id] = song
    }
  }
  return filteredData
}

export const addTracks = (data: unknown, ids?: Array<string | number>) => {
  const songs = filterSongs(data, ids)
  return {
    type: PLAYER_ADD_TRACKS,
    data: songs,
  }
}

export const playNext = (data: unknown, ids?: Array<string | number>) => {
  const songs = filterSongs(data, ids)
  return {
    type: PLAYER_PLAY_NEXT,
    data: songs,
  }
}

export const shuffle = (data) => {
  const ids = Object.keys(data)
  for (let i = ids.length - 1; i > 0; i--) {
    let j = Math.floor(Math.random() * (i + 1))
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
  }
  const shuffled = {}
  // The "_" is to force the object key to be a string, so it keeps the order when adding to object
  // or else the keys will always be in the same (numerically) order
  ids.forEach((id) => (shuffled['_' + id] = data[id]))
  return shuffled
}

export const shuffleTracks = (data: unknown, ids?: Array<string | number>) => {
  const songs = filterSongs(data, ids)
  const shuffled = shuffle(songs)
  const firstId = Object.keys(shuffled)[0]
  return {
    type: PLAYER_PLAY_TRACKS,
    id: firstId,
    data: shuffled,
  }
}

export const playTracks = (
  data: unknown,
  ids?: Array<string | number>,
  selectedId?: string | number,
) => {
  const songs = filterSongs(data, ids)
  return {
    type: PLAYER_PLAY_TRACKS,
    id: selectedId || Object.keys(songs)[0],
    data: songs,
  }
}

export const syncQueue = (audioInfo, audioLists) => ({
  type: PLAYER_SYNC_QUEUE,
  data: {
    audioInfo,
    audioLists,
  },
})

export const clearQueue = () => ({
  type: PLAYER_CLEAR_QUEUE,
})

export const currentPlaying = (audioInfo) => ({
  type: PLAYER_CURRENT,
  data: audioInfo,
})

export const setVolume = (volume) => ({
  type: PLAYER_SET_VOLUME,
  data: { volume },
})

export const setPlayMode = (mode) => ({
  type: PLAYER_SET_MODE,
  data: { mode },
})

export const setTranscodingProfile = (profile) => ({
  type: TRANSCODING_SET_PROFILE,
  data: profile,
})

export const refreshQueue = (resolvedUrls) => ({
  type: PLAYER_REFRESH_QUEUE,
  data: resolvedUrls,
})
