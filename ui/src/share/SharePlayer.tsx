import ReactJkMusicPlayer from 'navidrome-music-player'
import { useCallback, useEffect, useRef, useState, type ComponentProps } from 'react'
import config, { shareInfo } from '../config'
import { shareCoverUrl, shareDownloadUrl, shareStreamUrl } from '../utils'
import Box from '@mui/material/Box'

// How long the download button stays inert after a click. The browser needs a
// moment to show its own download UI; until then the page looks unresponsive.
export const DOWNLOAD_FEEDBACK_MS = 5000

const SharePlayer = () => {
  const [downloading, setDownloading] = useState(false)
  const timer = useRef<number | null>(null)

  useEffect(
    () => () => {
      if (timer.current !== null) {
        window.clearTimeout(timer.current)
      }
    },
    [],
  )

  const list =
    shareInfo?.tracks.map((s) => {
    return {
      name: s.title,
      musicSrc: shareStreamUrl(s.id),
      cover: shareCoverUrl(s.id, true),
      singer: s.artist,
      duration: s.duration,
    }
  })
  // An anchor, not a navigation: the service worker's NavigationRoute would
  // intercept the streamed archive and fail it.
  const customDownloader = useCallback(() => {
    const link = document.createElement('a')
    link.href = shareDownloadUrl(shareInfo?.id)
    link.download = ''
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)

    setDownloading(true)
    clearTimeout(timer.current ?? undefined)
    timer.current = window.setTimeout(
      () => setDownloading(false),
      DOWNLOAD_FEEDBACK_MS,
    )
  }, [])
  const options = {
    audioLists: list ?? [],
    mode: 'full',
    toggleMode: false,
    mobileMediaQuery: '',
    showDownload: shareInfo?.downloadable && config.enableDownloads,
    showReload: false,
    showMediaSession: true,
    theme: 'auto',
    showThemeSwitch: false,
    restartCurrentOnPrev: true,
    remove: false,
    spaceBar: true,
    volumeFade: { fadeIn: 200, fadeOut: 200 },
    sortableOptions: { delay: 200, delayOnTouchOnly: true },
  }
  return (
    <Box
      sx={{
        '& .group .next-audio': {
          pointerEvents: shareInfo?.tracks.length === 1 ? 'none' : undefined,
          opacity: shareInfo?.tracks.length === 1 ? 0.65 : undefined,
        },
        '& .group.audio-download': {
          pointerEvents: downloading ? 'none' : undefined,
          opacity: downloading ? 0.65 : undefined,
        },
        '@media (min-width: 768px)': {
          '& .react-jinke-music-player-mobile > div': {
            width: 768,
            margin: 'auto',
          },
          '& .react-jinke-music-player-mobile-cover': {
            width: 'auto !important',
          },
        },
      }}
    >
      <ReactJkMusicPlayer
        {...(options as ComponentProps<typeof ReactJkMusicPlayer>)}
        customDownloader={customDownloader}
      />
    </Box>
  )
}

export default SharePlayer
