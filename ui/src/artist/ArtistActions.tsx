import React from 'react'
import { useDispatch } from 'react-redux'
import { useMediaQuery, CircularProgress } from '@mui/material'
import type { SxProps, Theme } from '@mui/material/styles'
import {
  Button,
  TopToolbar,
  sanitizeListRestProps,
  useDataProvider,
  useNotify,
  useTranslate,
} from 'react-admin'
import ShuffleIcon from '@mui/icons-material/Shuffle'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import ShareIcon from '@mui/icons-material/Share'
import CloudDownloadOutlinedIcon from '@mui/icons-material/CloudDownloadOutlined'
import { IoIosRadio } from 'react-icons/io'
import { playShuffle, playTopSongs } from './actions'
import { playSimilar } from '../common/playbackActions'
import {
  openShareMenu,
  openDownloadMenu,
  DOWNLOAD_MENU_ARTIST,
} from '../actions'
import config from '../config'
import { formatBytes } from '../utils'
import { artistDownloadSize } from '../common/artist'
import type { ArtistRecord } from '../types/records'

const toolbarSx = {
  minHeight: 'auto',
  p: '0 !important',
  background: 'transparent',
  boxShadow: 'none',
  '& .MuiToolbar-root': {
    minHeight: 'auto',
    p: '0 !important',
    background: 'transparent',
  },
}

const buttonSx = (theme) => ({
  [theme.breakpoints.down('sm')]: {
    minWidth: 'auto',
    p: '8px 12px',
    fontSize: '0.75rem',
    '& .MuiButton-startIcon': { mr: '4px' },
  },
})

type LoadingButtonProps = {
  loading?: boolean
  icon?: React.ReactNode
  onClick?: () => void
  label?: string
  sx?: SxProps<Theme>
  size?: 'small' | 'medium' | 'large'
  disabled?: boolean
}

const LoadingButton = ({
  loading,
  icon,
  onClick,
  label,
  sx,
  size,
  disabled,
}: LoadingButtonProps) => (
  <Button
    onClick={onClick}
    label={label}
    sx={sx}
    size={size}
    disabled={disabled}
  >
    {loading ? <CircularProgress size={20} color="inherit" /> : icon}
  </Button>
)

type ArtistActionsProps = {
  className?: string
  record: ArtistRecord
}

const ArtistActions = ({
  className = '',
  record,
  ...rest
}: ArtistActionsProps) => {
  const dispatch = useDispatch()
  const translate = useTranslate()
  const dataProvider = useDataProvider()
  const notify = useNotify()
  const isMobile = useMediaQuery((theme) => theme.breakpoints.down('sm'))
  const [loadingAction, setLoadingAction] = React.useState<string | null>(null)
  const isLoading = !!loadingAction

  const albumArtistSize = artistDownloadSize(record)
  const hasAlbumArtistContent = Boolean(albumArtistSize)

  const handlePlay = React.useCallback(async () => {
    setLoadingAction('play')
    try {
      await playTopSongs(dispatch, notify, record.name!)
    } catch (e) {
      // eslint-disable-next-line no-console
      console.error('Error fetching top songs for artist:', e)
      notify('ra.page.error', { type: 'warning' })
    } finally {
      setLoadingAction(null)
    }
  }, [dispatch, notify, record.name])

  const handleShuffle = React.useCallback(async () => {
    setLoadingAction('shuffle')
    try {
      await playShuffle(dataProvider, dispatch, record.id)
    } catch (e) {
      // eslint-disable-next-line no-console
      console.error('Error fetching songs for shuffle:', e)
      notify('ra.page.error', { type: 'warning' })
    } finally {
      setLoadingAction(null)
    }
  }, [dataProvider, dispatch, record.id, notify])

  const handleRadio = React.useCallback(async () => {
    setLoadingAction('radio')
    try {
      await playSimilar(dispatch, notify, String(record.id))
    } catch (e) {
      // eslint-disable-next-line no-console
      console.error('Error starting radio for artist:', e)
      notify('ra.page.error', { type: 'warning' })
    } finally {
      setLoadingAction(null)
    }
  }, [dispatch, notify, record.id])

  const handleShare = React.useCallback(() => {
    dispatch(openShareMenu([record.id], 'artist', record.name, record.name))
  }, [dispatch, record])

  const handleDownload = React.useCallback(() => {
    dispatch(openDownloadMenu(record, DOWNLOAD_MENU_ARTIST))
  }, [dispatch, record])

  return (
    <TopToolbar
      className={className}
      sx={toolbarSx}
      {...sanitizeListRestProps(rest)}
    >
      <LoadingButton
        onClick={handlePlay}
        label={translate('resources.artist.actions.topSongs')}
        sx={buttonSx}
        size={isMobile ? 'small' : 'medium'}
        disabled={isLoading}
        loading={loadingAction === 'play'}
        icon={<PlayArrowIcon />}
      />
      <LoadingButton
        onClick={handleShuffle}
        label={translate('resources.artist.actions.shuffle')}
        sx={buttonSx}
        size={isMobile ? 'small' : 'medium'}
        disabled={isLoading}
        loading={loadingAction === 'shuffle'}
        icon={<ShuffleIcon />}
      />
      <LoadingButton
        onClick={handleRadio}
        label={translate('resources.artist.actions.radio')}
        sx={buttonSx}
        size={isMobile ? 'small' : 'medium'}
        disabled={isLoading}
        loading={loadingAction === 'radio'}
        icon={
          <IoIosRadio style={isMobile ? { fontSize: '1.5rem' } : undefined} />
        }
      />
      {config.enableSharing && hasAlbumArtistContent && (
        <LoadingButton
          onClick={handleShare}
          label={translate('ra.action.share')}
          sx={buttonSx}
          size={isMobile ? 'small' : 'medium'}
          icon={<ShareIcon />}
        />
      )}
      {config.enableDownloads && hasAlbumArtistContent && (
        <LoadingButton
          onClick={handleDownload}
          label={`${translate('ra.action.download')} (${formatBytes(
            albumArtistSize,
          )})`}
          sx={buttonSx}
          size={isMobile ? 'small' : 'medium'}
          icon={<CloudDownloadOutlinedIcon />}
        />
      )}
    </TopToolbar>
  )
}

export default ArtistActions
