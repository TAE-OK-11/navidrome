// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, {
  isValidElement,
  useMemo,
  useCallback,
  useState,
  forwardRef,
} from 'react'
import { useDispatch } from 'react-redux'
import {
  Datagrid,
  PureDatagridBody,
  PureDatagridRow,
  useRecordContext,
  useTranslate,
} from 'react-admin'
import { TableCell, TableRow, Typography, useMediaQuery } from '@mui/material'
import PropTypes from 'prop-types'
import makeStyles from '../themes/makeStyles'
import AlbumIcon from '@mui/icons-material/Album'
import clsx from 'clsx'
import { useDrag } from 'react-dnd'
import Lightbox from 'react-image-lightbox'
import 'react-image-lightbox/style.css'
import { playTracks } from '../actions'
import subsonic from '../subsonic'
import { AlbumContextMenu } from '../common'
import { DraggableTypes } from '../consts'
import { formatFullDate } from '../utils'

const useStyles = makeStyles({
  subtitle: {
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    verticalAlign: 'middle',
    display: 'flex',
    alignItems: 'center',
  },
  discIcon: {
    marginRight: '14px',
  },
  discCoverArt: {
    width: '48px',
    height: '48px',
    marginRight: '14px',
    objectFit: 'cover',
    borderRadius: '4px',
    flexShrink: 0,
    cursor: 'pointer',
  },
  row: {
    cursor: 'pointer',
    transition: 'background-color 150ms ease, transform 150ms ease',
    '&:hover': {
      backgroundColor: 'rgba(127, 127, 127, 0.1)',
      '& $contextMenu': {
        visibility: 'visible',
      },
    },
    '&:active': { transform: 'scale(0.998)' },
  },
  missingRow: {
    cursor: 'inherit',
    opacity: 0.3,
  },
  headerStyle: {
    border: '1px solid rgba(127, 127, 127, 0.2)',
    borderRadius: 16,
    overflow: 'hidden',
    '& thead': {
      boxShadow: '0px 3px 8px rgba(0, 0, 0, 0.08)',
      backgroundColor: 'rgba(127, 127, 127, 0.08)',
    },
    '& th': {
      fontWeight: 'bold',
      padding: '13px 15px',
      letterSpacing: '0.02em',
    },
  },
  contextMenu: {
    visibility: (props) => (props.isDesktop ? 'hidden' : 'visible'),
  },
})

export const DiscSubtitleRow = forwardRef(
  ({ record, onClick, colSpan, contextAlwaysVisible }, ref) => {
    const translate = useTranslate()
    const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'))
    const classes = useStyles({ isDesktop })
    const [imageError, setImageError] = useState(false)
    const [isLightboxOpen, setLightboxOpen] = useState(false)
    const lightboxClosedAt = React.useRef(0)
    const handlePlaySubset = (discNumber) => () => {
      // Ignore clicks shortly after the lightbox was closed to prevent
      // mobile touch events from "falling through" the overlay and
      // triggering playback.
      if (Date.now() - lightboxClosedAt.current < 400) {
        return
      }
      onClick(discNumber)
    }

    const coverArtUrl = subsonic.getDiscCoverArtUrl(
      record.albumId,
      record.discNumber,
      record.updatedAt,
      96,
    )

    const fullImageUrl = subsonic.getDiscCoverArtUrl(
      record.albumId,
      record.discNumber,
      record.updatedAt,
    )

    const handleOpenLightbox = useCallback(
      (e) => {
        if (!imageError) {
          e.stopPropagation()
          setLightboxOpen(true)
        }
      },
      [imageError],
    )

    const handleCloseLightbox = useCallback(() => {
      lightboxClosedAt.current = Date.now()
      setLightboxOpen(false)
    }, [])

    const subtitle = record.discSubtitle
      ? record.discSubtitle
      : translate('resources.song.fields.disc', {
          discNumber: record.discNumber,
        })

    return (
      <TableRow
        hover
        ref={ref}
        onClick={handlePlaySubset(record.discNumber)}
        className={classes.row}
      >
        <TableCell colSpan={colSpan}>
          <Typography variant="h6" className={classes.subtitle}>
            {!imageError ? (
              <img
                src={coverArtUrl}
                className={classes.discCoverArt}
                alt=""
                onClick={handleOpenLightbox}
                onError={() => setImageError(true)}
              />
            ) : (
              <AlbumIcon className={classes.discIcon} fontSize={'small'} />
            )}
            {subtitle}
          </Typography>
          {isLightboxOpen && !imageError && (
            // Lightbox portals out of the row, but React still bubbles its
            // events up this tree, where the row's onClick would play the disc.
            <span onClick={(e) => e.stopPropagation()}>
              <Lightbox
                imagePadding={50}
                animationDuration={200}
                imageTitle={record.album + ' - ' + subtitle}
                mainSrc={fullImageUrl}
                onCloseRequest={handleCloseLightbox}
              />
            </span>
          )}
        </TableCell>
        <TableCell>
          <AlbumContextMenu
            record={{ id: record.albumId }}
            discNumber={record.discNumber}
            showLove={false}
            className={classes.contextMenu}
            hideShare={true}
            hideInfo={true}
            visible={contextAlwaysVisible}
          />
        </TableCell>
      </TableRow>
    )
  },
)

DiscSubtitleRow.displayName = 'DiscSubtitleRow'

export const SongDatagridRow = ({
  record: recordOverride,
  children,
  firstTracksOfDiscs,
  contextAlwaysVisible,
  onClickSubset = () => {},
  className,
  ...rest
}) => {
  const record = useRecordContext({ record: recordOverride })
  const classes = useStyles()
  const fields = React.Children.toArray(children).filter((c) =>
    isValidElement(c),
  )

  const [, dragDiscRef] = useDrag(
    () => ({
      type: DraggableTypes.DISC,
      item: {
        discs: [
          {
            albumId: record?.albumId,
            discNumber: record?.discNumber,
          },
        ],
      },
      options: { dropEffect: 'copy' },
    }),
    [record],
  )

  const [, dragSongRef] = useDrag(
    () => ({
      type: DraggableTypes.SONG,
      item: { ids: [record?.mediaFileId || record?.id] },
      options: { dropEffect: 'copy' },
    }),
    [record],
  )

  if (!record || !record.title) {
    return null
  }

  const rowClick = record.missing ? undefined : rest.rowClick

  const computedClasses = clsx(
    className,
    classes.row,
    record.missing && classes.missingRow,
  )
  const childCount = fields.length
  return (
    <>
      {firstTracksOfDiscs.has(record.id) && (
        <DiscSubtitleRow
          ref={dragDiscRef}
          record={record}
          onClick={onClickSubset}
          contextAlwaysVisible={contextAlwaysVisible}
          colSpan={childCount + (rest.expand ? 1 : 0)}
        />
      )}
      <PureDatagridRow
        ref={dragSongRef}
        record={record}
        {...rest}
        rowClick={rowClick}
        className={computedClasses}
      >
        {fields}
      </PureDatagridRow>
    </>
  )
}

SongDatagridRow.propTypes = {
  record: PropTypes.object,
  children: PropTypes.node,
  firstTracksOfDiscs: PropTypes.instanceOf(Set),
  contextAlwaysVisible: PropTypes.bool,
  onClickSubset: PropTypes.func,
}

const SongDatagridBody = ({
  contextAlwaysVisible,
  showDiscSubtitles,
  ...rest
}) => {
  const dispatch = useDispatch()
  const records = rest.data || []
  const ids = records.map((record) => record.id)
  const dataById = Object.fromEntries(
    records.map((record) => [record.id, record]),
  )

  const playSubset = useCallback(
    (discNumber) => {
      let idsToPlay = []
      if (discNumber !== undefined) {
        idsToPlay = ids.filter((id) => dataById[id].discNumber === discNumber)
      }
      dispatch(
        playTracks(
          dataById,
          idsToPlay?.filter((id) => !dataById[id].missing),
        ),
      )
    },
    [dispatch, dataById, ids],
  )

  const firstTracksOfDiscs = useMemo(() => {
    if (!ids) {
      return new Set()
    }
    let foundSubtitle = false
    const set = new Set(
      ids
        .filter((i) => dataById[i])
        .reduce((acc, id) => {
          const last = acc && acc[acc.length - 1]
          foundSubtitle = foundSubtitle || dataById[id].discSubtitle
          if (
            acc.length === 0 ||
            (last && dataById[id].discNumber !== dataById[last].discNumber)
          ) {
            acc.push(id)
          }
          return acc
        }, []),
    )
    if (!showDiscSubtitles || (set.size < 2 && !foundSubtitle)) {
      set.clear()
    }
    return set
  }, [ids, dataById, showDiscSubtitles])

  return (
    <PureDatagridBody
      {...rest}
      row={
        <SongDatagridRow
          firstTracksOfDiscs={firstTracksOfDiscs}
          contextAlwaysVisible={contextAlwaysVisible}
          onClickSubset={playSubset}
        />
      }
    />
  )
}

export const SongDatagrid = ({
  contextAlwaysVisible,
  showDiscSubtitles,
  ...rest
}) => {
  const classes = useStyles()
  return (
    <Datagrid
      className={classes.headerStyle}
      isRowSelectable={(r) => !r?.missing}
      {...rest}
      body={
        <SongDatagridBody
          contextAlwaysVisible={contextAlwaysVisible}
          showDiscSubtitles={showDiscSubtitles}
        />
      }
    />
  )
}

SongDatagrid.propTypes = {
  contextAlwaysVisible: PropTypes.bool,
  showDiscSubtitles: PropTypes.bool,
  classes: PropTypes.object,
}
