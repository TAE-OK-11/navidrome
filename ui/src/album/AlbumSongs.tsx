import React, { useMemo } from 'react'
import {
  BulkActionsToolbar,
  FunctionField,
  ListToolbar,
  NumberField,
  TextField,
  useListContext,
} from 'react-admin'
import { useDispatch, useSelector } from 'react-redux'
import { Box, Card, useMediaQuery } from '@mui/material'
import FavoriteBorderIcon from '@mui/icons-material/FavoriteBorder'
import { playTracks } from '../actions'
import {
  ArtistLinkField,
  DateField,
  DurationField,
  QualityInfo,
  RatingField,
  SizeField,
  SongBulkActions,
  SongContextMenu,
  SongDatagrid,
  SongInfo,
  SongTitleField,
  useResourceRefresh,
  useSelectedFields,
} from '../common'
import config from '../config'
import ExpandInfoDialog from '../dialogs/ExpandInfoDialog'
import { removeAlbumCommentsFromSongs } from './utils'
import { componentStyleOverride } from '../themes/componentStyleOverride'
import type { AppState } from '../types/redux'
import type { AlbumRecord, SongRecord } from '../types/records'

const contentSx = (bulkActionsDisplayed) => (theme) => ({
  mt: bulkActionsDisplayed ? -8 : 0,
  transition: theme.transitions.create('margin-top'),
  position: 'relative',
  flex: '1 1 auto',
  [theme.breakpoints.down('sm')]: { boxShadow: 'none' },
  ...componentStyleOverride(theme, 'RaList', 'content'),
  ...(bulkActionsDisplayed
    ? componentStyleOverride(theme, 'RaList', 'bulkActionsDisplayed')
    : {}),
})

const AlbumSongs = (props: {
  data?: SongRecord[]
  actions?: React.ReactElement
  album?: AlbumRecord
  selectedIds?: string[]
  resource?: string
  exporter?: boolean
}) => {
  const records = props.data || []
  const ids = records.map((record) => record.id)
  const dataById = Object.fromEntries(
    records.map((record) => [record.id, record]),
  )
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'))
  const dispatch = useDispatch()
  const version = useSelector(
    (state: AppState) => state.activity?.refresh?.lastReceived || 0,
  )
  useResourceRefresh('song', 'album')

  const toggleableFields = useMemo(() => {
    return {
      trackNumber: isDesktop && (
        <TextField source="trackNumber" label="#" sortable={false} />
      ),
      title: (
        <SongTitleField
          source="title"
          sortable={false}
          showTrackNumbers={!isDesktop}
        />
      ),
      artist: isDesktop && <ArtistLinkField source="artist" sortable={false} />,
      composer: isDesktop && (
        <ArtistLinkField source="composer" sortable={false} />
      ),
      duration: <DurationField source="duration" sortable={false} />,
      year: isDesktop && (
        <FunctionField
          source="year"
          render={(r) => r.year || ''}
          sortable={false}
        />
      ),
      playCount: isDesktop && (
        <NumberField source="playCount" sortable={false} />
      ),
      playDate: <DateField source="playDate" sortable={false} showTime />,
      quality: isDesktop && <QualityInfo source="quality" sortable={false} />,
      size: isDesktop && <SizeField source="size" sortable={false} />,
      channels: isDesktop && <NumberField source="channels" sortable={false} />,
      bpm: isDesktop && <NumberField source="bpm" sortable={false} />,
      genre: <TextField source="genre" sortable={false} />,
      mood: isDesktop && (
        <FunctionField
          source="mood"
          render={(r) => r.tags?.mood?.[0] ?? ''}
          sortable={false}
        />
      ),
      rating: isDesktop && config.enableStarRating && (
        <RatingField
          resource={'song'}
          source="rating"
          sortable={false}
          sx={{ visibility: 'hidden' }}
        />
      ),
    }
  }, [isDesktop])

  const columns = useSelectedFields({
    resource: 'albumSong',
    columns: toggleableFields,
    omittedColumns: ['title'],
    defaultOff: [
      'composer',
      'channels',
      'bpm',
      'year',
      'playCount',
      'playDate',
      'size',
      'mood',
      'genre',
    ],
  })

  const bulkActionsLabel = isDesktop
    ? 'ra.action.bulk_actions'
    : 'ra.action.bulk_actions_mobile'

  return (
    <>
      <ListToolbar
        sx={{ justifyContent: 'flex-start' }}
        actions={props.actions ?? undefined}
        {...(props as Record<string, unknown>)}
      />
      <Box sx={{ display: 'flex' }}>
        <Card
          sx={contentSx((props.selectedIds?.length ?? 0) > 0)}
          key={version}
        >
          <BulkActionsToolbar {...props} label={bulkActionsLabel}>
            <SongBulkActions />
          </BulkActionsToolbar>
          <SongDatagrid
            rowClick={(id) => dispatch(playTracks(dataById, ids, id))}
            {...props}
            hasBulkActions={true}
            showDiscSubtitles={true}
            contextAlwaysVisible={!isDesktop}
          >
            {columns}
            <SongContextMenu
              source={'starred'}
              sortable={false}
              sx={{ visibility: isDesktop ? 'hidden' : 'visible' }}
              label={
                config.enableFavourites && (
                  <FavoriteBorderIcon
                    fontSize={'small'}
                    sx={{ ml: '3px', mt: '-2px', verticalAlign: 'text-top' }}
                  />
                )
              }
            />
          </SongDatagrid>
        </Card>
      </Box>
      <ExpandInfoDialog content={(<SongInfo />) as React.ReactElement} />
    </>
  )
}

const SanitizedAlbumSongs = (props: {
  actions?: React.ReactElement
  album?: AlbumRecord
  resource?: string
  exporter?: boolean
}) => {
  const context = useListContext()
  removeAlbumCommentsFromSongs({ album: props.album, data: context.data })
  return (
    <>
      {!context.isPending && (
        <AlbumSongs
          data={context.data}
          selectedIds={context.selectedIds}
          actions={props.actions}
          album={props.album}
        />
      )}
    </>
  )
}

export default SanitizedAlbumSongs
