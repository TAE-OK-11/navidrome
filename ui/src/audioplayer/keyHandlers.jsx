const keyHandlers = (audioInstance, queue, current) => {
  const nextSong = () => {
    const idx = queue.findIndex((item) => item.uuid === current?.uuid)
    return idx >= 0 ? queue[idx + 1] : null
  }

  const prevSong = () => {
    const idx = queue.findIndex((item) => item.uuid === current?.uuid)
    return idx >= 0 ? queue[idx - 1] : null
  }

  return {
    TOGGLE_PLAY: (e) => {
      e.preventDefault()
      audioInstance && audioInstance.togglePlay()
    },
    VOL_UP: () =>
      (audioInstance.volume = Math.min(1, audioInstance.volume + 0.1)),
    VOL_DOWN: () =>
      (audioInstance.volume = Math.max(0, audioInstance.volume - 0.1)),
    PREV_SONG: (e) => {
      if (!e.metaKey && prevSong()) audioInstance && audioInstance.playPrev()
    },
    CURRENT_SONG: () => {
      if (current?.song?.albumId) {
        window.location.href = `#/album/${current.song.albumId}/show`
      }
    },
    NEXT_SONG: (e) => {
      if (!e.metaKey && nextSong()) audioInstance && audioInstance.playNext()
    },
  }
}

export default keyHandlers
