// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useState, useEffect, useCallback, useRef } from 'react'
import PropTypes from 'prop-types'
import { useSelector, useDispatch } from 'react-redux'
import { useTranslate, Link, useNotify } from 'react-admin'
import {
  Popover,
  IconButton,
  Tooltip,
  List,
  ListItem,
  Avatar,
  Badge,
  Card,
  CardContent,
  Typography,
  LinearProgress,
  Box,
  useTheme,
  useMediaQuery,
} from '@mui/material'
import { FaRegCirclePlay, FaPause } from 'react-icons/fa6'
import subsonic from '../subsonic'
import { useInterval } from '../common'
import { nowPlayingCountSync } from '../actions'
import { formatDuration } from '../utils'
import config from '../config'

const ellipsisSx = {
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
}

// NowPlayingButton component - handles the button with badge
const NowPlayingButton = React.memo(({ count, onClick }) => {
  const translate = useTranslate()

  return (
    <Tooltip title={translate('nowPlaying.title')}>
      <IconButton
        sx={{ color: 'inherit' }}
        onClick={onClick}
        aria-label={translate('nowPlaying.title')}
        aria-haspopup="true"
        size="large"
      >
        <Badge
          badgeContent={count}
          color="primary"
          overlap="rectangular"
          sx={(theme) => ({
            '& .MuiBadge-badge': {
              backgroundColor: theme.palette.primary.main,
              color: theme.palette.primary.contrastText,
            },
          })}
        >
          <FaRegCirclePlay size={20} />
        </Badge>
      </IconButton>
    </Tooltip>
  )
})

NowPlayingButton.displayName = 'NowPlayingButton'

NowPlayingButton.propTypes = {
  count: PropTypes.number.isRequired,
  onClick: PropTypes.func.isRequired,
}

const NowPlayingItem = React.memo(
  ({ nowPlayingEntry, onLinkClick, getArtistLink, now }) => {
    const isPaused = nowPlayingEntry.state === 'paused'
    const isPlaying =
      nowPlayingEntry.state === 'playing' ||
      nowPlayingEntry.state === 'starting'
    const basePositionMs = nowPlayingEntry.positionMs || 0
    const rate = nowPlayingEntry.playbackRate || 1
    const elapsedSinceFetch = now - (nowPlayingEntry._fetchedAt || now)
    const interpolatedMs = isPlaying
      ? basePositionMs + elapsedSinceFetch * rate
      : basePositionMs
    const durationMs = (nowPlayingEntry.duration || 0) * 1000
    const clampedMs = Math.max(0, interpolatedMs)
    const positionMs =
      durationMs > 0 ? Math.min(clampedMs, durationMs) : clampedMs
    const positionSec = positionMs / 1000
    const durationSec = nowPlayingEntry.duration || 0
    const progress = durationSec > 0 ? (positionSec / durationSec) * 100 : 0
    const artistId = nowPlayingEntry.albumArtistId || nowPlayingEntry.artistId
    const artistName = nowPlayingEntry.albumArtist || nowPlayingEntry.artist

    return (
      <ListItem sx={{ alignItems: 'flex-start', gap: 1.5, p: 1 }}>
        <Box
          sx={{ position: 'relative', flexShrink: 0, width: 64, height: 64 }}
        >
          <Link
            to={`/album/${nowPlayingEntry.albumId}/show`}
            onClick={onLinkClick}
          >
            <Avatar
              sx={{
                width: '100%',
                height: '100%',
                cursor: 'pointer',
                borderRadius: 0.5,
                '&:hover': { opacity: 0.8 },
              }}
              src={subsonic.getCoverArtUrl(nowPlayingEntry, 80)}
              variant="square"
              alt={`${nowPlayingEntry.album} cover art`}
              loading="lazy"
            />
          </Link>
          {isPaused && (
            <Box
              sx={{
                position: 'absolute',
                inset: 0,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                backgroundColor: 'rgba(0, 0, 0, 0.45)',
                borderRadius: 0.5,
                pointerEvents: 'none',
              }}
            >
              <Box
                component={FaPause}
                sx={{ color: 'rgba(255, 255, 255, 0.85)', fontSize: 18 }}
              />
            </Box>
          )}
        </Box>
        <Box
          sx={{
            flex: 1,
            minWidth: 0,
            display: 'flex',
            flexDirection: 'column',
            gap: 0.25,
          }}
        >
          <Typography
            sx={{
              fontWeight: 600,
              fontSize: '0.875rem',
              lineHeight: 1.3,
              ...ellipsisSx,
            }}
            title={nowPlayingEntry.title}
          >
            {nowPlayingEntry.title}
          </Typography>
          {artistId ? (
            <Box
              component={Link}
              to={getArtistLink(artistId)}
              onClick={onLinkClick}
              sx={(theme) => ({
                cursor: 'pointer',
                color: theme.palette.text.secondary,
                fontSize: '0.75rem',
                '&:hover': { textDecoration: 'underline' },
              })}
            >
              {artistName}
            </Box>
          ) : (
            <Typography
              sx={(theme) => ({
                fontSize: '0.75rem',
                color: theme.palette.text.secondary,
                ...ellipsisSx,
              })}
            >
              {artistName}
            </Typography>
          )}
          <Typography
            sx={(theme) => ({
              fontSize: '0.75rem',
              color: theme.palette.text.secondary,
              ...ellipsisSx,
            })}
            title={nowPlayingEntry.album}
          >
            {nowPlayingEntry.album}
          </Typography>
          <Box
            sx={{ display: 'flex', alignItems: 'center', gap: 0.75, mt: 0.5 }}
          >
            <Box
              component="span"
              sx={(theme) => ({
                fontSize: '0.65rem',
                color: theme.palette.text.secondary,
                fontVariantNumeric: 'tabular-nums',
                flexShrink: 0,
              })}
            >
              {formatDuration(positionSec)}
            </Box>
            <LinearProgress
              sx={(theme) => ({
                flex: 1,
                height: 3,
                borderRadius: 0.5,
                backgroundColor: theme.palette.action.disabledBackground,
                '& .MuiLinearProgress-bar': { borderRadius: 0.5 },
              })}
              variant="determinate"
              value={Math.min(progress, 100)}
            />
            <Box
              component="span"
              sx={(theme) => ({
                fontSize: '0.65rem',
                color: theme.palette.text.secondary,
                fontVariantNumeric: 'tabular-nums',
                flexShrink: 0,
              })}
            >
              {formatDuration(durationSec)}
            </Box>
          </Box>
          <Typography
            sx={(theme) => ({
              fontSize: '0.65rem',
              color: theme.palette.text.disabled,
              mt: 0.25,
            })}
          >
            {nowPlayingEntry.username}
            {nowPlayingEntry.playerName
              ? ` (${nowPlayingEntry.playerName})`
              : ''}
          </Typography>
        </Box>
      </ListItem>
    )
  },
)

NowPlayingItem.displayName = 'NowPlayingItem'

NowPlayingItem.propTypes = {
  nowPlayingEntry: PropTypes.shape({
    playerId: PropTypes.oneOfType([PropTypes.string, PropTypes.number])
      .isRequired,
    albumId: PropTypes.oneOfType([PropTypes.string, PropTypes.number])
      .isRequired,
    albumArtistId: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
    artistId: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
    albumArtist: PropTypes.string,
    artist: PropTypes.string,
    title: PropTypes.string.isRequired,
    username: PropTypes.string.isRequired,
    playerName: PropTypes.string,
    album: PropTypes.string,
    state: PropTypes.string,
    positionMs: PropTypes.number,
    duration: PropTypes.number,
  }).isRequired,
  onLinkClick: PropTypes.func.isRequired,
  getArtistLink: PropTypes.func.isRequired,
  now: PropTypes.number.isRequired,
}

// NowPlayingList component - handles the popover content
const NowPlayingList = React.memo(
  ({ anchorEl, open, onClose, entries, onLinkClick, getArtistLink, now }) => {
    const translate = useTranslate()

    return (
      <Popover
        id="panel-nowplaying"
        anchorEl={anchorEl}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
        open={open}
        onClose={onClose}
        aria-labelledby="now-playing-title"
      >
        <Card sx={{ p: 0 }}>
          <CardContent sx={{ p: '8px !important', pb: '8px !important' }}>
            {entries.length === 0 ? (
              <Typography id="now-playing-title">
                {translate('nowPlaying.empty')}
              </Typography>
            ) : (
              <List
                sx={{
                  width: '26em',
                  maxHeight:
                    entries.length > 0
                      ? `${Math.min(entries.length, 3) * 120}px`
                      : '12em',
                  overflowY: 'auto',
                  p: 0,
                }}
                dense
                aria-label={translate('nowPlaying.title')}
              >
                {entries.map((nowPlayingEntry) => (
                  <NowPlayingItem
                    key={`${nowPlayingEntry.username}-${nowPlayingEntry.playerName}`}
                    nowPlayingEntry={nowPlayingEntry}
                    onLinkClick={onLinkClick}
                    getArtistLink={getArtistLink}
                    now={now}
                  />
                ))}
              </List>
            )}
          </CardContent>
        </Card>
      </Popover>
    )
  },
)

NowPlayingList.displayName = 'NowPlayingList'

NowPlayingList.propTypes = {
  anchorEl: PropTypes.object,
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  entries: PropTypes.arrayOf(PropTypes.object).isRequired,
  onLinkClick: PropTypes.func.isRequired,
  getArtistLink: PropTypes.func.isRequired,
  now: PropTypes.number.isRequired,
}

// Main NowPlayingPanel component
const NowPlayingPanel = () => {
  const dispatch = useDispatch()
  const count = useSelector((state) => state.activity.nowPlayingCount)
  const lastUpdate = useSelector((state) => state.activity.nowPlayingLastUpdate)
  const streamReconnected = useSelector(
    (state) => state.activity.streamReconnected,
  )
  const serverUp = useSelector(
    (state) => !!state.activity.serverStart.startTime,
  )
  const translate = useTranslate()
  const notify = useNotify()
  const theme = useTheme()
  const isSmallScreen = useMediaQuery(theme.breakpoints.down('md'))

  const [anchorEl, setAnchorEl] = useState(null)
  const [entries, setEntries] = useState([])
  const [now, setNow] = useState(Date.now())
  const open = Boolean(anchorEl)

  const handleMenuOpen = useCallback((event) => {
    setAnchorEl(event.currentTarget)
  }, [])

  const handleMenuClose = useCallback(() => {
    setAnchorEl(null)
  }, [])

  // Close panel when link is clicked on small screens
  const handleLinkClick = useCallback(() => {
    if (isSmallScreen) {
      handleMenuClose()
    }
  }, [isSmallScreen, handleMenuClose])

  const getArtistLink = useCallback((artistId) => {
    if (!artistId) return null
    return config.devShowArtistPage && artistId !== config.variousArtistsId
      ? `/artist/${artistId}/show`
      : `/album?filter={"artist_id":"${artistId}"}&order=ASC&sort=max_year&displayedFilters={"compilation":true}&perPage=15`
  }, [])

  const fetchTimerRef = useRef(null)
  const doFetchRef = useRef()
  doFetchRef.current = () =>
    subsonic
      .getNowPlaying()
      .then((resp) => resp.json['subsonic-response'])
      .then((data) => {
        if (data.status === 'ok') {
          const nowPlayingEntries = data.nowPlaying?.entry || []
          const fetchTime = Date.now()
          setEntries(
            nowPlayingEntries.map((e) => ({ ...e, _fetchedAt: fetchTime })),
          )
          dispatch(nowPlayingCountSync({ count: nowPlayingEntries.length }))
        } else {
          throw new Error(
            data.error?.message || 'Failed to fetch now playing data',
          )
        }
      })
      .catch((error) => {
        notify('ra.page.error', {
          type: 'warning',
          messageArgs: { error: error.message || 'Unknown error' },
        })
      })
  const fetchList = useCallback(() => {
    if (fetchTimerRef.current) clearTimeout(fetchTimerRef.current)
    fetchTimerRef.current = setTimeout(() => {
      fetchTimerRef.current = null
      doFetchRef.current()
    }, 300)
  }, [])

  useEffect(() => {
    return () => {
      if (fetchTimerRef.current) clearTimeout(fetchTimerRef.current)
    }
  }, [])

  // Initialize count and entries on mount, and refresh on server/stream changes
  useEffect(() => {
    if (serverUp) fetchList()
  }, [fetchList, serverUp, streamReconnected])

  // Refresh when NowPlaying updates from SSE events (if panel is open)
  useEffect(() => {
    if (open && serverUp) fetchList()
  }, [lastUpdate, open, fetchList, serverUp])

  // Update current time every second when open to animate progress bars
  useInterval(() => setNow(Date.now()), open ? 1000 : null)

  // Periodic refresh when panel is open (10 seconds)
  useInterval(
    () => {
      if (open && serverUp) fetchList()
    },
    open ? 10000 : null,
  )

  // Periodic refresh when panel is closed (60 seconds) to keep badge accurate
  useInterval(
    () => {
      if (!open && serverUp) fetchList()
    },
    !open ? 60000 : null,
  )

  return (
    <Box>
      <NowPlayingButton count={count} onClick={handleMenuOpen} />
      <NowPlayingList
        anchorEl={anchorEl}
        open={open}
        onClose={handleMenuClose}
        entries={entries}
        now={now}
        onLinkClick={handleLinkClick}
        getArtistLink={getArtistLink}
      />
    </Box>
  )
}

NowPlayingPanel.propTypes = {}

export default NowPlayingPanel
