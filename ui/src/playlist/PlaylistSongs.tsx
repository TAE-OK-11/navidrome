import React, { useCallback, useEffect, useMemo } from 'react'
import {
  BulkActionsToolbar,
  ListToolbar,
  TextField,
  NumberField,
  useDataProvider,
  useNotify,
  useListContext,
  FunctionField,
} from 'react-admin'
import { useDispatch, useSelector } from 'react-redux'
import { Box, Card, useMediaQuery } from '@mui/material'
import ReactDragListView from 'react-drag-listview'
import {
  DurationField,
  SongInfo,
  SongContextMenu,
  SongDatagrid,
  SongTitleField,
  QualityInfo,
  useSelectedFields,
  useResourceRefresh,
  DateField,
  ArtistLinkField,
  RatingField,
} from '../common'
import { AlbumLinkField } from '../song/AlbumLinkField'
import { playTracks } from '../actions'
import PlaylistSongBulkActions from './PlaylistSongBulkActions'
import ExpandInfoDialog from '../dialogs/ExpandInfoDialog'
import config from '../config'
import { componentStyleOverride } from '../themes/componentStyleOverride'
import type { AppState } from '../types/redux'
import type { Identifier } from 'react-admin'
import type { SongRecord } from '../types/records'

const contentSx = (bulkActionsDisplayed: boolean) => (theme) => ({
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

const ReorderableList = ({
  readOnly,
  children,
  ...rest
}: {
  readOnly?: boolean
  children: React.ReactNode
  onDragEnd?: (from: number, to: number) => void
  nodeSelector?: string
}) => {
  if (readOnly) {
    return children
  }
  return <ReactDragListView {...(rest as React.ComponentProps<typeof ReactDragListView>)}>{children}</ReactDragListView>
}

type PlaylistSongsProps = {
  playlistId?: Identifier
  readOnly?: boolean
  actions?: React.ReactElement
  filters?: React.ReactElement
  pagination?: React.ReactElement
  resource?: string
  exporter?: boolean
}

const PlaylistSongs = ({
  playlistId,
  readOnly,
  actions,
  ...props
}: PlaylistSongsProps) => {
  const listContext = useListContext<SongRecord>()
  const {
    data = [],
    selectedIds,
    onUnselectItems,
    refetch,
    setPage,
  } = listContext
  const ids = data.map((record) => record.id)
  const dataById = Object.fromEntries(data.map((record) => [record.id, record]))
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'))
  const dispatch = useDispatch()
  const dataProvider = useDataProvider()
  const notify = useNotify()
  const version = useSelector(
    (state: AppState) => state.activity?.refresh?.lastReceived || 0,
  )
  useResourceRefresh('song', 'playlist')

  useEffect(() => {
    setPage(1)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [playlistId, setPage])

  const onAddToPlaylist = useCallback(() => {
    refetch()
  }, [refetch])

  const reorder = useCallback(
    (playlistId: Identifier, id: Identifier, newPos: Identifier) => {
      dataProvider
        .update('playlistTrack', {
          id,
          data: { insert_before: newPos },
          previousData: dataById[id],
          filter: { playlist_id: playlistId },
        })
        .then(() => {
          refetch()
        })
        .catch(() => {
          notify('ra.page.error', { type: 'warning' })
        })
    },
    [dataById, dataProvider, notify, refetch],
  )

  const handleDragEnd = useCallback(
    (from: number, to: number) => {
      const toId = ids[to]
      const fromId = ids[from]
      reorder(playlistId!, fromId, toId)
    },
    [playlistId, reorder, ids],
  )

  const toggleableFields = useMemo(() => {
    return {
      trackNumber: isDesktop && <TextField source="id" label={'#'} />,
      title: <SongTitleField source="title" showTrackNumbers={false} />,
      album: isDesktop && <AlbumLinkField source="album" />,
      artist: isDesktop && <ArtistLinkField source="artist" />,
      albumArtist: isDesktop && <ArtistLinkField source="albumArtist" />,
      duration: <DurationField source="duration" />,
      year: isDesktop && (
        <FunctionField
          source="year"
          render={(r) => r.year || ''}
          sortByOrder={'DESC'}
        />
      ),
      playCount: isDesktop && (
        <NumberField source="playCount" sortByOrder={'DESC'} />
      ),
      playDate: isDesktop && (
        <DateField source="playDate" sortByOrder={'DESC'} showTime />
      ),
      quality: isDesktop && <QualityInfo source="quality" sortable={false} />,
      channels: isDesktop && <NumberField source="channels" />,
      bpm: isDesktop && <NumberField source="bpm" />,
      genre: <TextField source="genre" />,
      rating: config.enableStarRating && (
        <RatingField
          source="rating"
          sortByOrder={'DESC'}
          resource={'song'}
          sx={{ visibility: 'hidden' }}
        />
      ),
    }
  }, [isDesktop])

  const columns = useSelectedFields({
    resource: 'playlistTrack',
    columns: toggleableFields,
    defaultOff: [
      'channels',
      'bpm',
      'year',
      'playCount',
      'playDate',
      'albumArtist',
      'genre',
      'rating',
    ],
  })

  return (
    <>
      <ListToolbar
        sx={{ justifyContent: 'flex-start' }}
        filters={props.filters}
        actions={actions}
      />
      <Box sx={{ display: 'flex' }}>
        <Card sx={contentSx(selectedIds.length > 0)} key={version}>
          <BulkActionsToolbar>
            <PlaylistSongBulkActions
              playlistId={playlistId}
              onUnselectItems={onUnselectItems}
              readOnly={readOnly}
              resource="playlistTrack"
            />
          </BulkActionsToolbar>
          <ReorderableList
            readOnly={readOnly}
            onDragEnd={handleDragEnd}
            nodeSelector={'tr'}
          >
            <SongDatagrid
              rowClick={(id) => dispatch(playTracks(dataById, ids, id))}
              {...listContext}
              hasBulkActions={!readOnly}
              contextAlwaysVisible={!isDesktop}
            >
              {columns}
              <SongContextMenu
                onAddToPlaylist={onAddToPlaylist}
                showLove={true}
                sx={{ visibility: isDesktop ? 'hidden' : 'visible' }}
              />
            </SongDatagrid>
          </ReorderableList>
        </Card>
      </Box>
      <ExpandInfoDialog content={<SongInfo /> as React.ReactElement} />
      {props.pagination &&
        React.cloneElement(
          props.pagination,
          listContext as unknown as Record<string, unknown>,
        )}
    </>
  )
}

const SanitizedPlaylistSongs = (props: {
  id?: Identifier
  actions?: React.ReactElement
  pagination?: React.ReactElement
  readOnly?: boolean
  resource?: string
  exporter?: boolean
}) => {
  const { isPending } = useListContext()
  return (
    <>
      {!isPending && (
        <PlaylistSongs
          playlistId={props.id}
          actions={props.actions}
          pagination={props.pagination}
          readOnly={props.readOnly}
          resource={props.resource}
          exporter={props.exporter}
        />
      )}
    </>
  )
}

export default SanitizedPlaylistSongs
