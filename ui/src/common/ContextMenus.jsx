import React, { useState } from 'react'
import PropTypes from 'prop-types'
import { useDispatch } from 'react-redux'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import { MdQuestionMark } from 'react-icons/md'
import makeStyles from '../themes/makeStyles'
import {
  useDataProvider,
  useNotify,
  useRecordContext,
  useTranslate,
} from 'react-admin'
import clsx from 'clsx'
import {
  playNext,
  addTracks,
  playTracks,
  shuffleTracks,
  openAddToPlaylist,
  openDownloadMenu,
  openExtendedInfoDialog,
  DOWNLOAD_MENU_ALBUM,
  DOWNLOAD_MENU_ARTIST,
  openShareMenu,
} from '../actions'
import { LoveButton } from './LoveButton'
import config from '../config'
import { formatBytes } from '../utils'
import { artistDownloadSize } from './artist'

const useStyles = makeStyles({
  noWrap: {
    whiteSpace: 'nowrap',
  },
  menu: {
    color: (props) => props.color,
  },
})

const MoreButton = ({ record, onClick, info, ...rest }) => {
  const handleClick = record.missing
    ? (e) => {
        e.preventDefault()
        info.action(record)
        e.stopPropagation()
      }
    : onClick
  return (
    <IconButton onClick={handleClick} size={'small'} {...rest}>
      {record?.missing ? (
        <MdQuestionMark fontSize={'large'} />
      ) : (
        <MoreVertIcon fontSize={'small'} />
      )}
    </IconButton>
  )
}

const ContextMenu = ({
  resource,
  showLove,
  record,
  color,
  className,
  songQueryParams,
  hideShare,
  hideInfo,
}) => {
  const classes = useStyles({ color })
  const dataProvider = useDataProvider()
  const dispatch = useDispatch()
  const translate = useTranslate()
  const notify = useNotify()
  const [anchorEl, setAnchorEl] = useState(null)

  const isArtist = resource === 'artist'
  const downloadSize = isArtist ? artistDownloadSize(record) : record?.size

  const options = {
    play: {
      enabled: true,
      needData: true,
      label: translate('resources.album.actions.playAll'),
      action: (data, ids) => dispatch(playTracks(data, ids)),
    },
    playNext: {
      enabled: true,
      needData: true,
      label: translate('resources.album.actions.playNext'),
      action: (data, ids) => dispatch(playNext(data, ids)),
    },
    addToQueue: {
      enabled: true,
      needData: true,
      label: translate('resources.album.actions.addToQueue'),
      action: (data, ids) => dispatch(addTracks(data, ids)),
    },
    shuffle: {
      enabled: true,
      needData: true,
      label: translate('resources.album.actions.shuffle'),
      action: (data, ids) => dispatch(shuffleTracks(data, ids)),
    },
    addToPlaylist: {
      enabled: true,
      needData: true,
      label: translate('resources.album.actions.addToPlaylist'),
      action: (data, ids) => dispatch(openAddToPlaylist({ selectedIds: ids })),
    },
    ...(!hideShare && {
      share: {
        enabled: config.enableSharing && (!isArtist || downloadSize),
        needData: false,
        label: translate('ra.action.share'),
        action: (record) =>
          dispatch(openShareMenu([record.id], resource, record.name)),
      },
    }),
    download: {
      enabled: config.enableDownloads && downloadSize,
      needData: false,
      label: `${translate('ra.action.download')} (${formatBytes(downloadSize)})`,
      action: () => {
        dispatch(
          openDownloadMenu(
            record,
            record.duration !== undefined
              ? DOWNLOAD_MENU_ALBUM
              : DOWNLOAD_MENU_ARTIST,
          ),
        )
      },
    },
    ...(!hideInfo && {
      info: {
        enabled: true,
        needData: true,
        label: translate('resources.album.actions.info'),
        action: () => dispatch(openExtendedInfoDialog(record)),
      },
    }),
  }

  const handleClick = (e) => {
    e.preventDefault()
    setAnchorEl(e.currentTarget)
    e.stopPropagation()
  }

  const handleOnClose = (e) => {
    e.preventDefault()
    setAnchorEl(null)
    e.stopPropagation()
  }

  let extractSongsData = function (response) {
    const data = response.data.reduce(
      (acc, cur) => ({ ...acc, [cur.id]: cur }),
      {},
    )
    const ids = response.data.map((r) => r.id)
    return { data, ids }
  }

  const handleItemClick = (e) => {
    setAnchorEl(null)
    const key = e.currentTarget.dataset.action
    if (options[key].needData) {
      dataProvider
        .getList('song', songQueryParams)
        .then((response) => {
          let { data, ids } = extractSongsData(response)
          options[key].action(data, ids)
        })
        .catch(() => {
          notify('ra.page.error', { type: 'warning' })
        })
    } else {
      options[key].action(record)
    }

    e.stopPropagation()
  }

  const open = Boolean(anchorEl)

  if (!record) {
    return null
  }

  const present = !record.missing

  return (
    <span className={clsx(classes.noWrap, className)}>
      <LoveButton
        record={record}
        resource={resource}
        visible={config.enableFavourites && showLove && present}
        color={color}
      />
      <MoreButton
        record={record}
        onClick={handleClick}
        info={options.info}
        aria-label="more"
        aria-controls="context-menu"
        aria-haspopup="true"
        className={classes.menu}
      />
      <Menu
        id="context-menu"
        anchorEl={anchorEl}
        keepMounted
        open={open}
        onClose={handleOnClose}
      >
        {Object.keys(options).map(
          (key) =>
            options[key].enabled && (
              <MenuItem data-action={key} key={key} onClick={handleItemClick}>
                {options[key].label}
              </MenuItem>
            ),
        )}
      </Menu>
    </span>
  )
}

export const AlbumContextMenu = ({
  showLove = true,
  record: recordOverride,
  ...props
}) => {
  const record = useRecordContext({ record: recordOverride })
  return record ? (
    <ContextMenu
      {...props}
      record={record}
      showLove={showLove}
      resource={'album'}
      songQueryParams={{
        pagination: { page: 1, perPage: -1 },
        sort: { field: 'album', order: 'ASC' },
        filter: {
          album_id: record.id,
          disc_number: props.discNumber,
          missing: false,
        },
      }}
    />
  ) : null
}

AlbumContextMenu.propTypes = {
  record: PropTypes.object,
  discNumber: PropTypes.number,
  color: PropTypes.string,
  showLove: PropTypes.bool,
}

export const ArtistContextMenu = ({
  showLove = true,
  record: recordOverride,
  ...props
}) => {
  const record = useRecordContext({ record: recordOverride })
  return record ? (
    <ContextMenu
      {...props}
      record={record}
      showLove={showLove}
      hideInfo={true}
      resource={'artist'}
      songQueryParams={{
        pagination: { page: 1, perPage: 200 },
        sort: {
          field: 'album',
          order: 'ASC',
        },
        filter: { album_artist_id: record.id, missing: false },
      }}
    />
  ) : null
}

ArtistContextMenu.propTypes = {
  record: PropTypes.object,
  color: PropTypes.string,
  showLove: PropTypes.bool,
}
