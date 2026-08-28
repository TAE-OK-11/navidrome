// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import { useDispatch } from 'react-redux'
import {
  Button,
  sanitizeListRestProps,
  TopToolbar,
  useListContext,
  useRecordContext,
  useTranslate,
} from 'react-admin'
import { Box, useMediaQuery } from '@mui/material'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import ShuffleIcon from '@mui/icons-material/Shuffle'
import CloudDownloadOutlinedIcon from '@mui/icons-material/CloudDownloadOutlined'
import { RiPlayListAddFill, RiPlayList2Fill } from 'react-icons/ri'
import PlaylistAddIcon from '@mui/icons-material/PlaylistAdd'
import ShareIcon from '@mui/icons-material/Share'
import {
  playNext,
  addTracks,
  playTracks,
  shuffleTracks,
  openAddToPlaylist,
  openDownloadMenu,
  DOWNLOAD_MENU_ALBUM,
  openShareMenu,
} from '../actions'
import { formatBytes } from '../utils'
import config from '../config'
import { ToggleFieldsMenu } from '../common'

const AlbumButton = ({ children, ...rest }) => {
  const record = useRecordContext(rest) || {}
  return (
    <Button {...rest} disabled={record.missing}>
      {children}
    </Button>
  )
}

const AlbumActions = ({ className, record = {}, permanentFilter, ...rest }) => {
  const dispatch = useDispatch()
  const translate = useTranslate()
  const { data: records = [] } = useListContext()
  const ids = records.map((song) => song.id)
  const data = Object.fromEntries(records.map((song) => [song.id, song]))
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'))
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))

  const handlePlay = React.useCallback(() => {
    dispatch(playTracks(data, ids))
  }, [dispatch, data, ids])

  const handlePlayNext = React.useCallback(() => {
    dispatch(playNext(data, ids))
  }, [dispatch, data, ids])

  const handlePlayLater = React.useCallback(() => {
    dispatch(addTracks(data, ids))
  }, [dispatch, data, ids])

  const handleShuffle = React.useCallback(() => {
    dispatch(shuffleTracks(data, ids))
  }, [dispatch, data, ids])

  const handleAddToPlaylist = React.useCallback(() => {
    const selectedIds = ids.filter((id) => !data[id].missing)
    dispatch(openAddToPlaylist({ selectedIds }))
  }, [dispatch, data, ids])

  const handleShare = React.useCallback(() => {
    dispatch(openShareMenu([record.id], 'album', record.name))
  }, [dispatch, record])

  const handleDownload = React.useCallback(() => {
    dispatch(openDownloadMenu(record, DOWNLOAD_MENU_ALBUM))
  }, [dispatch, record])

  return (
    <TopToolbar className={className} {...sanitizeListRestProps(rest)}>
      <Box
        sx={{ display: 'flex', justifyContent: 'space-between', width: '100%' }}
      >
        <Box>
          <AlbumButton
            onClick={handlePlay}
            label={translate('resources.album.actions.playAll')}
          >
            <PlayArrowIcon />
          </AlbumButton>
          <AlbumButton
            onClick={handleShuffle}
            label={translate('resources.album.actions.shuffle')}
          >
            <ShuffleIcon />
          </AlbumButton>
          <AlbumButton
            onClick={handlePlayNext}
            label={translate('resources.album.actions.playNext')}
          >
            <RiPlayList2Fill />
          </AlbumButton>
          <AlbumButton
            onClick={handlePlayLater}
            label={translate('resources.album.actions.addToQueue')}
          >
            <RiPlayListAddFill />
          </AlbumButton>
          <AlbumButton
            onClick={handleAddToPlaylist}
            label={translate('resources.album.actions.addToPlaylist')}
          >
            <PlaylistAddIcon />
          </AlbumButton>
          {config.enableSharing && (
            <AlbumButton
              onClick={handleShare}
              label={translate('ra.action.share')}
            >
              <ShareIcon />
            </AlbumButton>
          )}
          {config.enableDownloads && (
            <AlbumButton
              onClick={handleDownload}
              label={
                translate('ra.action.download') +
                (isDesktop ? ` (${formatBytes(record.size)})` : '')
              }
            >
              <CloudDownloadOutlinedIcon />
            </AlbumButton>
          )}
        </Box>
        <Box>{isNotSmall && <ToggleFieldsMenu resource="albumSong" />}</Box>
      </Box>
    </TopToolbar>
  )
}

export default AlbumActions
