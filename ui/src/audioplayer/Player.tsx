import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useInterval } from '../common'
import { useDispatch, useSelector } from 'react-redux'
import { Box, useMediaQuery } from '@mui/material'
import {
  createTheme,
  ThemeProvider,
  StyledEngineProvider,
} from '@mui/material/styles'
import type { ThemeOptions } from '@mui/material/styles'
import { useAuthState, useDataProvider, useTranslate } from 'react-admin'
import ReactGA from 'react-ga4'
import { useAppHotkey } from '../hooks/useAppHotkey'
import ReactJkMusicPlayer from 'navidrome-music-player'
import 'navidrome-music-player/assets/index.css'

const NavidromeMusicPlayer =
  ReactJkMusicPlayer as unknown as React.ComponentType<Record<string, unknown>>
import useCurrentTheme from '../themes/useCurrentTheme'
import config from '../config'
import AudioTitle from './AudioTitle'
import {
  clearQueue,
  currentPlaying,
  refreshQueue,
  setPlayMode,
  setTranscodingProfile,
  setVolume,
  syncQueue,
} from '../actions'
import PlayerToolbar from './PlayerToolbar'
import { sendNotification } from '../utils'
import subsonic from '../subsonic'
import locale from './locale'
import keyHandlers from './keyHandlers'
import { calculateGain } from '../utils/calculateReplayGain'
import { detectBrowserProfile, decisionService } from '../transcode'
import modernizeTheme from '../themes/modernizeTheme'
import { componentStyleOverride } from '../themes/componentStyleOverride'
import type { NavidromeRootState } from '../types/redux'

const isMobileUserAgent =
  typeof navigator !== 'undefined' &&
  /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(
    navigator.userAgent,
  )

const Player = () => {
  const theme = useCurrentTheme()
  const muiTheme = useMemo(
    () => createTheme(modernizeTheme(theme) as ThemeOptions),
    [theme],
  )
  const translate = useTranslate()
  const playerTheme = theme.player?.theme || 'dark'
  const dataProvider = useDataProvider()
  const playerState = useSelector((state: NavidromeRootState) => state.player)
  const playerAutoPlay = playerState.autoPlay
  const playerClear = playerState.clear
  const playerCurrent = playerState.current
  const playerMode = playerState.mode
  const playerPlayIndex = playerState.playIndex
  const playerQueue = playerState.queue
  const playerSavedPlayIndex = playerState.savedPlayIndex
  const playerVolume = playerState.volume
  const dispatch = useDispatch()
  const [currentTrackId, setCurrentTrackId] = useState<string | null>(null)
  const [heartbeatTrackId, setHeartbeatTrackId] = useState<string | null>(null)
  const lastPositionMsRef = useRef(0)
  const currentTrackIdRef = useRef<string | null>(null)
  const lastStoppedTrackIdRef = useRef<string | null>(null)
  const stoppedRef = useRef(false)
  const [audioInstance, setAudioInstance] = useState<HTMLAudioElement | null>(
    null,
  )
  const isDesktop = useMediaQuery('(min-width:810px)')
  const isMobilePlayer = isMobileUserAgent

  const { authenticated } = useAuthState()

  // Keep a ref to playerState so the mount effect can read the latest value
  // without re-triggering on every queue/position change
  const playerStateRef = useRef(playerState)
  playerStateRef.current = playerState

  currentTrackIdRef.current = currentTrackId

  const reportStopped = useCallback((trackId: string, posMs: number) => {
    if (lastStoppedTrackIdRef.current === trackId) {
      return
    }
    lastStoppedTrackIdRef.current = trackId
    subsonic.reportPlayback(trackId, posMs, 'stopped')
  }, [])

  useInterval(
    () => {
      if (heartbeatTrackId && !stoppedRef.current) {
        subsonic.reportPlayback(
          heartbeatTrackId,
          lastPositionMsRef.current,
          'playing',
        )
      }
    },
    heartbeatTrackId ? config.playbackReportIntervalMs : null,
  )

  // Detect browser codec profile and eagerly resolve transcode URLs for the
  // persisted queue once on mount (e.g. after a browser refresh)
  useEffect(() => {
    const profile = detectBrowserProfile()
    decisionService.setProfile(profile)
    dispatch(setTranscodingProfile(profile))

    const state = playerStateRef.current
    const currentIdx = state.savedPlayIndex || 0
    const trackIds = state.queue
      .slice(currentIdx, currentIdx + 4)
      .filter((item) => !item.isRadio && item.trackId)
      .map((item) => item.trackId)

    if (trackIds.length === 0) {
      dispatch(refreshQueue(undefined))
      return
    }

    Promise.allSettled(
      trackIds.map((id) =>
        decisionService.resolveStreamUrl(id).then((url) => [id, url]),
      ),
    ).then((results) => {
      const resolvedUrls = {}
      results.forEach((r) => {
        if (r.status === 'fulfilled') {
          resolvedUrls[r.value[0]] = r.value[1]
        }
      })
      dispatch(refreshQueue(resolvedUrls))
    })
  }, [dispatch])

  // Pre-fetch transcode decisions for next 2-3 songs when queue or position changes
  useEffect(() => {
    if (!playerQueue.length) return

    const currentIdx = playerSavedPlayIndex || 0
    const nextSongIds = playerQueue
      .slice(currentIdx + 1, currentIdx + 4)
      .filter((item) => !item.isRadio)
      .map((item) => item.trackId)

    if (nextSongIds.length > 0) {
      decisionService.prefetchDecisions(nextSongIds)
    }
  }, [playerQueue, playerSavedPlayIndex])

  const visible = authenticated && playerQueue.length > 0
  const isRadio = playerCurrent?.isRadio || false
  const showNotifications = useSelector(
    (state: NavidromeRootState) => state.settings.notifications || false,
  )
  const gainInfo = useSelector((state: NavidromeRootState) => state.replayGain)
  const [context, setContext] = useState<AudioContext | null>(null)
  const [gainNode, setGainNode] = useState<GainNode | null>(null)

  useEffect(() => {
    if (
      context === null &&
      audioInstance &&
      config.enableReplayGain &&
      'AudioContext' in window &&
      (gainInfo.gainMode === 'album' || gainInfo.gainMode === 'track')
    ) {
      const ctx = new AudioContext()
      // we need this to support radios in firefox
      audioInstance.crossOrigin = 'anonymous'
      const source = ctx.createMediaElementSource(audioInstance)
      const gain = ctx.createGain()

      source.connect(gain)
      gain.connect(ctx.destination)

      setContext(ctx)
      setGainNode(gain)
    }
  }, [audioInstance, context, gainInfo.gainMode])

  useEffect(() => {
    if (gainNode) {
      const current = playerCurrent || {}
      const song = current.song || {}

      const numericGain = calculateGain(gainInfo, song)
      gainNode.gain.setValueAtTime(numericGain, context!.currentTime)
    }
  }, [audioInstance, context, gainNode, playerCurrent, gainInfo])

  useEffect(() => {
    const handleBeforeUnload = (e) => {
      if (
        playerStateRef.current?.current?.uuid &&
        audioInstance &&
        !audioInstance.paused
      ) {
        e.preventDefault()
        e.returnValue = ''
      }
    }

    const handlePageHide = () => {
      if (
        currentTrackIdRef.current &&
        !playerStateRef.current?.current?.isRadio
      ) {
        stoppedRef.current = true
        try {
          subsonic.reportPlaybackKeepalive(
            currentTrackIdRef.current,
            lastPositionMsRef.current,
            'stopped',
          )
        } catch {
          // fetch/sendBeacon may throw; ignore
        }
      }
    }

    window.addEventListener('beforeunload', handleBeforeUnload)
    window.addEventListener('pagehide', handlePageHide)
    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload)
      window.removeEventListener('pagehide', handlePageHide)
    }
  }, [audioInstance])

  const defaultOptions = useMemo(
    () => ({
      theme: playerTheme,
      bounds: 'body',
      playMode: playerMode,
      mode: 'full',
      loadAudioErrorPlayNext: false,
      autoPlayInitLoadPlayList: true,
      clearPriorAudioLists: false,
      showDestroy: true,
      showDownload: false,
      showLyric: true,
      showReload: false,
      toggleMode: !isDesktop,
      glassBg: false,
      showThemeSwitch: false,
      showMediaSession: true,
      restartCurrentOnPrev: true,
      quietUpdate: true,
      defaultPosition: {
        top: 300,
        left: 120,
      },
      volumeFade: { fadeIn: 200, fadeOut: 200 },
      renderAudioTitle: (audioInfo, isMobile) => (
        <AudioTitle
          audioInfo={audioInfo}
          gainInfo={gainInfo}
          isMobile={isMobile}
        />
      ),
      locale: locale(translate),
      sortableOptions: { delay: 200, delayOnTouchOnly: true },
    }),
    [gainInfo, isDesktop, playerTheme, translate, playerMode],
  )

  const options = useMemo(() => {
    const current = playerCurrent || {}
    return {
      ...defaultOptions,
      audioLists: playerQueue,
      playIndex: playerPlayIndex,
      autoPlay:
        playerQueue.length > 0 &&
        playerAutoPlay !== false &&
        (playerClear || playerPlayIndex === 0),
      clearPriorAudioLists: playerClear,
      extendsContent: (
        <PlayerToolbar id={current.trackId} isRadio={current.isRadio} />
      ),
      defaultVolume: isMobilePlayer ? 1 : playerVolume,
      showMediaSession: !current.isRadio,
    }
  }, [
    defaultOptions,
    isMobilePlayer,
    playerAutoPlay,
    playerClear,
    playerCurrent,
    playerPlayIndex,
    playerQueue,
    playerVolume,
  ])

  const onAudioListsChange = useCallback(
    (_, audioLists, audioInfo) => dispatch(syncQueue(audioInfo, audioLists)),
    [dispatch],
  )

  const onAudioProgress = useCallback((info) => {
    if (info.ended) {
      document.title = 'Navidrome'
    }
    if (!info.isRadio && info.currentTime != null) {
      lastPositionMsRef.current = Math.floor(info.currentTime * 1000)
    }
  }, [])

  const onAudioVolumeChange = useCallback(
    // sqrt to compensate for the logarithmic volume
    (volume) => dispatch(setVolume(Math.sqrt(volume))),
    [dispatch],
  )

  const onPlayModeChange = useCallback(
    (mode) => dispatch(setPlayMode(mode)),
    [dispatch],
  )

  const onAudioPlay = useCallback(
    (info) => {
      if (context && context.state !== 'running') {
        context.resume()
      }

      dispatch(currentPlaying(info))
      if (info.duration) {
        const song = info.song
        document.title = `${song.title} - ${song.artist} - Navidrome`
        if (!info.isRadio) {
          const posMs = Math.floor(info.currentTime * 1000)
          lastPositionMsRef.current = posMs
          const isNewTrack = info.trackId !== currentTrackId
          if (isNewTrack) {
            lastStoppedTrackIdRef.current = null
            subsonic
              .reportPlayback(info.trackId, posMs, 'starting')
              .then(() =>
                subsonic.reportPlayback(info.trackId, posMs, 'playing'),
              )
            setCurrentTrackId(info.trackId)
          } else {
            subsonic.reportPlayback(info.trackId, posMs, 'playing')
          }
          setHeartbeatTrackId(info.trackId)
        }
        if (config.gaTrackingId) {
          ReactGA.event({
            category: 'Player',
            action: 'Play song',
            label: `${song.title} - ${song.artist}`,
          })
        }
        if (showNotifications) {
          sendNotification(
            song.title,
            `${song.artist} - ${song.album}`,
            info.cover,
          )
        }
      }
    },
    [context, dispatch, showNotifications, currentTrackId],
  )

  const onAudioPlayTrackChange = useCallback(() => {
    if (currentTrackId) {
      reportStopped(currentTrackId, lastPositionMsRef.current)
    }
    setHeartbeatTrackId(null)
    setCurrentTrackId(null)
  }, [currentTrackId, reportStopped])

  const onAudioPause = useCallback(
    (info) => {
      dispatch(currentPlaying(info))
      if (!info.isRadio && currentTrackId) {
        const posMs = Math.floor(info.currentTime * 1000)
        lastPositionMsRef.current = posMs
        subsonic.reportPlayback(currentTrackId, posMs, 'paused')
      }
      setHeartbeatTrackId(null)
    },
    [dispatch, currentTrackId, reportStopped],
  )

  const onAudioEnded = useCallback(
    (currentPlayId, audioLists, info) => {
      if (currentTrackId && !info.isRadio) {
        const posMs = Math.floor((info.duration || 0) * 1000)
        reportStopped(currentTrackId, posMs)
      }
      setHeartbeatTrackId(null)
      setCurrentTrackId(null)
      dispatch(currentPlaying(info))
      dataProvider
        .getOne('keepalive', { id: info.trackId })
        // eslint-disable-next-line no-console
        .catch((e) => console.log('Keepalive error:', e))
    },
    [dispatch, dataProvider, currentTrackId, reportStopped],
  )

  const onCoverClick = useCallback((mode, audioLists, audioInfo) => {
    if (mode === 'full' && audioInfo?.song?.albumId) {
      window.location.href = `#/album/${audioInfo.song.albumId}/show`
    }
  }, [])

  const onAudioError = useCallback(
    (error, currentPlayId, audioLists, audioInfo) => {
      // Invalidate all cached decisions — token may be stale
      decisionService.invalidateAll()

      // Pre-fetch decisions for upcoming songs with fresh tokens
      const currentIdx = playerQueue.findIndex(
        (item) => item.uuid === currentPlayId,
      )
      if (currentIdx >= 0) {
        const nextSongIds = playerQueue
          .slice(currentIdx + 1, currentIdx + 4)
          .filter((item) => !item.isRadio)
          .map((item) => item.trackId)
        if (nextSongIds.length > 0) {
          decisionService.prefetchDecisions(nextSongIds)
        }
      }
    },
    [playerQueue],
  )

  const onBeforeDestroy = useCallback(() => {
    return new Promise((resolve, reject) => {
      if (currentTrackId && !playerStateRef.current?.current?.isRadio) {
        reportStopped(currentTrackId, lastPositionMsRef.current)
      }
      setHeartbeatTrackId(null)
      setCurrentTrackId(null)
      dispatch(clearQueue())
      reject()
    })
  }, [dispatch, currentTrackId])

  if (!visible) {
    document.title = 'Navidrome'
  }

  const handlers = useMemo(
    () => keyHandlers(audioInstance, playerQueue, playerCurrent),
    [audioInstance, playerQueue, playerCurrent],
  )

  useAppHotkey('TOGGLE_PLAY', handlers.TOGGLE_PLAY)
  useAppHotkey('VOL_UP', handlers.VOL_UP)
  useAppHotkey('VOL_DOWN', handlers.VOL_DOWN)
  useAppHotkey('PREV_SONG', handlers.PREV_SONG)
  useAppHotkey('NEXT_SONG', handlers.NEXT_SONG)
  useAppHotkey('CURRENT_SONG', handlers.CURRENT_SONG)

  useEffect(() => {
    if (isMobilePlayer && audioInstance) {
      audioInstance.volume = 1
    }
  }, [isMobilePlayer, audioInstance])

  // Report every seek (including programmatic ones the library does not surface
  // via onAudioSeeked, e.g. restartCurrentOnPrev). Debounce coalesces drag
  // bursts into one report at the final position.
  useEffect(() => {
    if (!audioInstance) return
    let timer: ReturnType<typeof setTimeout> | null = null
    const flush = () => {
      timer = null
      if (
        !currentTrackIdRef.current ||
        playerStateRef.current?.current?.isRadio
      ) {
        return
      }
      const posMs = Math.floor((audioInstance.currentTime || 0) * 1000)
      const state = audioInstance.paused ? 'paused' : 'playing'
      subsonic.reportPlayback(currentTrackIdRef.current, posMs, state)
    }
    const handleSeeked = () => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(flush, 250)
    }
    audioInstance.addEventListener('seeked', handleSeeked)
    return () => {
      if (timer) clearTimeout(timer)
      audioInstance.removeEventListener('seeked', handleSeeked)
    }
  }, [audioInstance])

  return (
    <StyledEngineProvider injectFirst>
      (
      <ThemeProvider theme={muiTheme}>
        <Box
          sx={[
            {
              display: visible ? 'block' : 'none',
              '@media screen and (max-width:810px)': {
                '& .sound-operation': { display: 'none' },
              },
              '@media (prefers-reduced-motion)': {
                '& .music-player-panel .panel-content div.img-rotate': {
                  animation: 'none',
                },
              },
              '& .progress-bar-content': {
                display: 'flex',
                flexDirection: 'column',
              },
              '& .play-mode-title': { pointerEvents: 'none' },
              '& .music-player-panel .panel-content div.img-rotate': {
                animationDuration: config.enableCoverAnimation
                  ? undefined
                  : '0s',
                borderRadius: config.enableCoverAnimation ? undefined : 0,
                backgroundSize: 'contain',
                backgroundPosition: 'center',
              },
              '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-cover':
                {
                  borderRadius: config.enableCoverAnimation ? undefined : 0,
                  width: config.enableCoverAnimation ? undefined : '85%',
                  maxWidth: config.enableCoverAnimation ? undefined : 600,
                  height: config.enableCoverAnimation ? undefined : 'auto',
                  aspectRatio: '1/1',
                  display: 'flex',
                },
              '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-cover img.cover':
                {
                  animationDuration: config.enableCoverAnimation
                    ? undefined
                    : '0s',
                  objectFit: 'contain',
                },
              '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-singer':
                { display: 'none' },
              '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-switch':
                { display: 'none' },
              '& .music-player-panel .panel-content .progress-bar-content section.audio-main':
                { display: isRadio ? 'none' : 'inline-flex' },
              '& .react-jinke-music-player-mobile-progress': {
                display: isRadio ? 'none' : 'flex',
              },
            },
            (theme) => componentStyleOverride(theme, 'NDAudioPlayer', 'player'),
          ]}
        >
          <NavidromeMusicPlayer
            {...options}
            onAudioListsChange={onAudioListsChange}
            onAudioVolumeChange={onAudioVolumeChange}
            onAudioProgress={onAudioProgress}
            onAudioPlay={onAudioPlay}
            onAudioPlayTrackChange={onAudioPlayTrackChange}
            onAudioPause={onAudioPause}
            onPlayModeChange={onPlayModeChange}
            onAudioEnded={onAudioEnded}
            onCoverClick={onCoverClick}
            onAudioError={onAudioError}
            onBeforeDestroy={onBeforeDestroy as () => Promise<void>}
            getAudioInstance={setAudioInstance}
          />
        </Box>
      </ThemeProvider>
      )
    </StyledEngineProvider>
  )
}

export { Player }
