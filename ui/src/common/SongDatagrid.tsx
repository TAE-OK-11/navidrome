import React, {
  isValidElement,
  useMemo,
  useCallback,
  useState,
  forwardRef,
  type ReactElement,
  type ReactNode,
} from 'react'
import { useDispatch } from 'react-redux'
import {
  Datagrid,
  PureDatagridBody,
  PureDatagridRow,
  useRecordContext,
  useTranslate,
  type Identifier,
  type RaRecord,
} from 'react-admin'
import { Box, TableCell, TableRow, Typography } from '@mui/material'
import AlbumIcon from '@mui/icons-material/Album'
import clsx from 'clsx'
import { useDrag } from 'react-dnd'
import ImageLightbox from './ImageLightbox'
import { playTracks } from '../actions'
import subsonic from '../subsonic'
import { AlbumContextMenu } from '../common'
import { DraggableTypes } from '../consts'
import type { SongRecord } from '../types/records'

const rowClass = 'nd-song-grid-row'
const missingRowClass = 'nd-song-grid-row-missing'

type DiscRecord = SongRecord & {
  albumId: Identifier
  discNumber: number
  discSubtitle?: string
  album?: string
}

type DiscSubtitleRowProps = {
  record: DiscRecord
  onClick: (discNumber: number) => void
  colSpan: number
  contextAlwaysVisible?: boolean
}

export const DiscSubtitleRow = forwardRef<
  HTMLTableRowElement,
  DiscSubtitleRowProps
>(({ record, onClick, colSpan, contextAlwaysVisible }, ref) => {
  const translate = useTranslate()
  const [imageError, setImageError] = useState(false)
  const [isLightboxOpen, setLightboxOpen] = useState(false)
  const lightboxClosedAt = React.useRef(0)
  const handlePlaySubset = (discNumber: number) => () => {
    // Ignore clicks shortly after the lightbox was closed to prevent
    // mobile touch events from "falling through" the overlay and
    // triggering playback.
    if (Date.now() - lightboxClosedAt.current < 400) {
      return
    }
    onClick(discNumber)
  }

  const coverArtUrl = subsonic.getDiscCoverArtUrl(
    String(record.albumId),
    record.discNumber,
    record.updatedAt,
    96,
  )

  const fullImageUrl = subsonic.getDiscCoverArtUrl(
    String(record.albumId),
    record.discNumber,
    record.updatedAt,
  )

  const handleOpenLightbox = useCallback(
    (e: React.MouseEvent) => {
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
      className={rowClass}
    >
      <TableCell colSpan={colSpan}>
        <Typography
          variant="h6"
          sx={{
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            verticalAlign: 'middle',
            display: 'flex',
            alignItems: 'center',
          }}
        >
          {!imageError ? (
            <Box
              component="img"
              src={coverArtUrl}
              sx={{
                width: 48,
                height: 48,
                mr: '14px',
                objectFit: 'cover',
                borderRadius: 1,
                flexShrink: 0,
                cursor: 'pointer',
              }}
              alt=""
              onClick={handleOpenLightbox}
              onError={() => setImageError(true)}
            />
          ) : (
            <AlbumIcon sx={{ mr: '14px' }} fontSize={'small'} />
          )}
          {subtitle}
        </Typography>
        <span onClick={(e) => e.stopPropagation()}>
          <ImageLightbox
            open={isLightboxOpen && !imageError}
            imageUrl={fullImageUrl}
            title={record.album + ' - ' + subtitle}
            onClose={handleCloseLightbox}
          />
        </span>
      </TableCell>
      <TableCell>
        <AlbumContextMenu
          record={{ id: record.albumId }}
          discNumber={record.discNumber}
          showLove={false}
          sx={{ visibility: contextAlwaysVisible ? 'visible' : 'hidden' }}
          hideShare={true}
          hideInfo={true}
          visible={contextAlwaysVisible}
        />
      </TableCell>
    </TableRow>
  )
})

DiscSubtitleRow.displayName = 'DiscSubtitleRow'

type SongDatagridRowProps = {
  record?: SongRecord
  children?: ReactNode
  firstTracksOfDiscs: Set<Identifier>
  contextAlwaysVisible?: boolean
  onClickSubset?: (discNumber?: number) => void
  className?: string
  rowClick?:
    | string
    | false
    | ((
        id: Identifier,
        resource: string,
        record: RaRecord,
      ) => string | false | void)
}

export const SongDatagridRow = ({
  record: recordOverride,
  children,
  firstTracksOfDiscs,
  contextAlwaysVisible,
  onClickSubset = () => {},
  className,
  ...rest
}: SongDatagridRowProps) => {
  const record = useRecordContext<SongRecord>({ record: recordOverride })
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
    rowClass,
    record.missing && missingRowClass,
  )
  const childCount = fields.length
  const rowExpand = (rest as { expand?: ReactNode }).expand
  return (
    <>
      {firstTracksOfDiscs.has(record.id) && (
        <DiscSubtitleRow
          ref={dragDiscRef as unknown as React.Ref<HTMLTableRowElement>}
          record={record as DiscRecord}
          onClick={onClickSubset}
          contextAlwaysVisible={contextAlwaysVisible}
          colSpan={childCount + (rowExpand ? 1 : 0)}
        />
      )}
      <PureDatagridRow
        ref={dragSongRef as unknown as React.Ref<HTMLTableRowElement>}
        record={record}
        {...rest}
        rowClick={rowClick as never}
        className={computedClasses}
      >
        {fields}
      </PureDatagridRow>
    </>
  )
}

type SongDatagridBodyProps = {
  contextAlwaysVisible?: boolean
  showDiscSubtitles?: boolean
  data?: SongRecord[]
  [key: string]: unknown
}

const DatagridBody = PureDatagridBody as React.ComponentType<
  SongDatagridBodyProps & { row?: ReactElement }
>

const SongDatagridBody = ({
  contextAlwaysVisible,
  showDiscSubtitles,
  ...rest
}: SongDatagridBodyProps) => {
  const dispatch = useDispatch()
  const records = rest.data || []
  const ids = records.map((record) => record.id)
  const dataById = Object.fromEntries(
    records.map((record) => [record.id, record]),
  ) as Record<string, SongRecord>

  const playSubset = useCallback(
    (discNumber?: number) => {
      let idsToPlay: Identifier[] = []
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
      return new Set<Identifier>()
    }
    let foundSubtitle = false
    const set = new Set(
      ids
        .filter((i) => dataById[i])
        .reduce<Identifier[]>((acc, id) => {
          const last = acc && acc[acc.length - 1]
          foundSubtitle = foundSubtitle || Boolean(dataById[id].discSubtitle)
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
    <DatagridBody
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

type SongDatagridProps = {
  contextAlwaysVisible?: boolean
  showDiscSubtitles?: boolean
  sx?: unknown
  [key: string]: unknown
}

export const SongDatagrid = ({
  contextAlwaysVisible,
  showDiscSubtitles,
  ...rest
}: SongDatagridProps) => {
  const { sx, ...datagridProps } = rest
  return (
    <Datagrid
      sx={[
        {
          border: '1px solid rgba(127, 127, 127, 0.2)',
          borderRadius: 4,
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
          [`& .${rowClass}`]: {
            cursor: 'pointer',
            transition: 'background-color 150ms ease, transform 150ms ease',
            '&:hover': {
              backgroundColor: 'rgba(127, 127, 127, 0.1)',
              '& .nd-song-context-menu, & .nd-rating-field': {
                visibility: 'visible',
              },
            },
            '&:active': { transform: 'scale(0.998)' },
          },
          [`& .${missingRowClass}`]: {
            cursor: 'inherit',
            opacity: 0.3,
          },
        },
        ...(Array.isArray(sx) ? sx : [sx]),
      ]}
      isRowSelectable={(r) => !r?.missing}
      {...datagridProps}
      body={
        <SongDatagridBody
          contextAlwaysVisible={contextAlwaysVisible}
          showDiscSubtitles={showDiscSubtitles}
        />
      }
    />
  )
}
