import React, { useState } from 'react'
import { useDispatch } from 'react-redux'
import {
  useNotify,
  usePermissions,
  useRecordContext,
  useTranslate,
  useDataProvider,
} from 'react-admin'
import { Box, IconButton, Menu, MenuItem } from '@mui/material'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import { MdQuestionMark } from 'react-icons/md'
import {
  playNext,
  addTracks,
  setTrack,
  openAddToPlaylist,
  openExtendedInfoDialog,
  openDownloadMenu,
  DOWNLOAD_MENU_SONG,
  openShareMenu,
} from '../actions'
import { LoveButton } from './LoveButton'
import config from '../config'
import { playSimilar } from './playbackActions'
import { formatBytes } from '../utils'
import { useRedirect } from 'react-admin'
import type { Identifier } from 'react-admin'
import type { SongRecord } from '../types/records'

type PlaylistSummary = {
  id: Identifier
  name: string
}

type ContextMenuRecord = SongRecord & {
  mediaFileId?: Identifier
  playlistId?: Identifier
  size?: number
  title?: string
  missing?: boolean
  rawTags?: Record<string, string[]>
}

type SongContextMenuProps = {
  resource?: string
  record?: ContextMenuRecord
  showLove?: boolean
  onAddToPlaylist?: (id?: Identifier) => void
  className?: string
  sx?: unknown
  source?: string
  sortable?: boolean
  sortByOrder?: string
  label?: React.ReactNode
}

type MoreButtonProps = {
  record: ContextMenuRecord
  onClick: (e: React.MouseEvent) => void
  info: { action: (record: ContextMenuRecord) => void }
}

const MoreButton = ({ record, onClick, info }: MoreButtonProps) => {
  const handleClick = record.missing
    ? (e: React.MouseEvent) => {
        info.action(record)
        e.stopPropagation()
      }
    : onClick
  return (
    <IconButton
      onClick={handleClick}
      size="small"
      aria-label="more"
      disabled={!record?.id}
    >
      {record?.missing ? (
        <MdQuestionMark fontSize={'large'} />
      ) : (
        <MoreVertIcon fontSize={'small'} />
      )}
    </IconButton>
  )
}

export const SongContextMenu = ({
  resource = 'song',
  record: recordOverride,
  showLove = true,
  onAddToPlaylist = () => {},
  className,
  sx,
}: SongContextMenuProps) => {
  const record = useRecordContext<ContextMenuRecord>({ record: recordOverride })
  const dispatch = useDispatch()
  const translate = useTranslate()
  const notify = useNotify()
  const dataProvider = useDataProvider()
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const [playlistAnchorEl, setPlaylistAnchorEl] = useState<HTMLElement | null>(
    null,
  )
  const [playlists, setPlaylists] = useState<PlaylistSummary[]>([])
  const [playlistsLoaded, setPlaylistsLoaded] = useState(false)
  const { permissions } = usePermissions()
  const redirect = useRedirect()

  if (!record?.id) {
    return null
  }

  const options = {
    playNow: {
      enabled: true,
      label: translate('resources.song.actions.playNow'),
      action: (menuRecord: ContextMenuRecord) => dispatch(setTrack(menuRecord)),
    },
    playNext: {
      enabled: true,
      label: translate('resources.song.actions.playNext'),
      action: (menuRecord: ContextMenuRecord) =>
        dispatch(playNext({ [menuRecord.id]: menuRecord })),
    },
    addToQueue: {
      enabled: true,
      label: translate('resources.song.actions.addToQueue'),
      action: (menuRecord: ContextMenuRecord) =>
        dispatch(addTracks({ [menuRecord.id]: menuRecord })),
    },
    instantMix: {
      enabled: config.enableExternalServices,
      label: translate('resources.song.actions.instantMix'),
      action: async (menuRecord: ContextMenuRecord) => {
        notify('message.startingInstantMix', { type: 'info' })
        try {
          const id = menuRecord.mediaFileId || menuRecord.id
          await playSimilar(dispatch, notify, String(id), {
            seedRecord: menuRecord,
            shuffle: false,
          })
        } catch (e) {
          // eslint-disable-next-line no-console
          console.error('Error starting instant mix:', e)
          notify('ra.page.error', { type: 'warning' })
        }
      },
    },
    addToPlaylist: {
      enabled: true,
      label: translate('resources.song.actions.addToPlaylist'),
      action: (menuRecord: ContextMenuRecord) =>
        dispatch(
          openAddToPlaylist({
            selectedIds: [menuRecord.mediaFileId || menuRecord.id],
            onSuccess: (id: Identifier) => onAddToPlaylist(id),
          }),
        ),
    },
    showInPlaylist: {
      enabled: true,
      label:
        translate('resources.song.actions.showInPlaylist') +
        (playlists.length > 0 ? ' ►' : ''),
      action: (menuRecord: ContextMenuRecord, e: React.MouseEvent) => {
        setPlaylistAnchorEl(e.currentTarget as HTMLElement)
      },
    },
    share: {
      enabled: config.enableSharing,
      label: translate('ra.action.share'),
      action: (menuRecord: ContextMenuRecord) =>
        dispatch(
          openShareMenu(
            [menuRecord.mediaFileId || menuRecord.id],
            'song',
            menuRecord.title,
            undefined,
          ),
        ),
    },
    download: {
      enabled: config.enableDownloads,
      label: `${translate('ra.action.download')} (${formatBytes(record.size ?? 0)})`,
      action: (menuRecord: ContextMenuRecord) =>
        dispatch(openDownloadMenu(menuRecord, DOWNLOAD_MENU_SONG)),
    },
    info: {
      enabled: true,
      label: translate('resources.song.actions.info'),
      action: async (menuRecord: ContextMenuRecord) => {
        let fullRecord = menuRecord
        if (permissions === 'admin' && !menuRecord.missing) {
          try {
            const id = menuRecord.mediaFileId ?? menuRecord.id
            const data = await dataProvider.inspect(id)
            fullRecord = { ...menuRecord, rawTags: data.data.rawTags }
          } catch (error) {
            const message =
              error instanceof Error ? error.message : String(error)
            notify(translate('ra.notification.http_error') + ': ' + message, {
              type: 'warning',
              multiLine: true,
              autoHideDuration: null,
            })
          }
        }

        dispatch(openExtendedInfoDialog(fullRecord))
      },
    },
  }

  const handleClick = (e: React.MouseEvent) => {
    setAnchorEl(e.currentTarget as HTMLElement)
    if (!playlistsLoaded) {
      const id = record.mediaFileId || record.id
      dataProvider
        .getPlaylists(id)
        .then((res) => {
          setPlaylists(res.data)
          setPlaylistsLoaded(true)
        })
        .catch((error) => {
          // eslint-disable-next-line no-console
          console.error('Failed to fetch playlists:', error)
          setPlaylists([])
          setPlaylistsLoaded(true)
        })
    }
    e.stopPropagation()
  }

  const handleClose = (e: React.MouseEvent) => {
    setAnchorEl(null)
    e.stopPropagation()
  }

  const handleItemClick = (e: React.MouseEvent<HTMLElement>) => {
    e.preventDefault()
    const key = e.currentTarget.dataset.action as string
    const action = options[key as keyof typeof options].action

    if (key === 'showInPlaylist') {
      // For showInPlaylist, we keep the main menu open and show submenu
      action(record, e)
    } else {
      // For other actions, close the main menu
      setAnchorEl(null)
      ;(action as (menuRecord: ContextMenuRecord) => void)(record)
    }
    e.stopPropagation()
  }

  const handlePlaylistClose = (
    _event?: object,
    _reason?: 'backdropClick' | 'escapeKeyDown',
  ) => {
    setPlaylistAnchorEl(null)
  }

  const handleMainMenuClose = (
    _event?: object,
    _reason?: 'backdropClick' | 'escapeKeyDown',
  ) => {
    setAnchorEl(null)
    setPlaylistAnchorEl(null) // Close both menus
  }

  const handlePlaylistClick = (id: Identifier, e: React.MouseEvent) => {
    e.stopPropagation()
    redirect(`/playlist/${id}/show`)
    handlePlaylistClose()
  }

  const present = !record.missing
  const open = Boolean(anchorEl)

  return (
    <Box
      component="span"
      className={`nd-song-context-menu${className ? ` ${className}` : ''}`}
      sx={[{ whiteSpace: 'nowrap' }, ...(Array.isArray(sx) ? sx : [sx])]}
    >
      <LoveButton
        record={record}
        resource={resource}
        visible={config.enableFavourites && showLove && present}
      />
      <MoreButton record={record} onClick={handleClick} info={options.info} />
      <Menu
        id={'menu' + record.id}
        anchorEl={anchorEl}
        open={open}
        onClose={handleMainMenuClose}
      >
        {Object.keys(options).map((key) => {
          const showInPlaylistDisabled =
            key === 'showInPlaylist' && !playlists.length
          return (
            options[key as keyof typeof options].enabled && (
              <MenuItem
                data-action={key}
                key={key}
                onClick={
                  showInPlaylistDisabled
                    ? (e) => e.stopPropagation()
                    : handleItemClick
                }
                onClickCapture={
                  showInPlaylistDisabled
                    ? (e) => e.stopPropagation()
                    : undefined
                }
                disabled={showInPlaylistDisabled}
                sx={
                  showInPlaylistDisabled ? { pointerEvents: 'auto' } : undefined
                }
              >
                {options[key as keyof typeof options].label}
              </MenuItem>
            )
          )
        })}
      </Menu>
      <Menu
        anchorEl={playlistAnchorEl}
        open={Boolean(playlistAnchorEl)}
        onClose={handlePlaylistClose}
        anchorOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'left',
        }}
      >
        {playlists.map((p) => (
          <MenuItem key={p.id} onClick={(e) => handlePlaylistClick(p.id, e)}>
            {p.name}
          </MenuItem>
        ))}
      </Menu>
    </Box>
  )
}
