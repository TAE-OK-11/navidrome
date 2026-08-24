// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
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

const ReorderableList = ({ readOnly, children, ...rest }) => {
  if (readOnly) {
    return children
  }
  return <ReactDragListView {...rest}>{children}</ReactDragListView>
}

const PlaylistSongs = ({ playlistId, readOnly, actions, ...props }) => {
  const listContext = useListContext()
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
    (state) => state.activity?.refresh?.lastReceived || 0,
  )
  useResourceRefresh('song', 'playlist')

  useEffect(() => {
    setPage(1)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [playlistId, setPage])

  const onAddToPlaylist = useCallback(
    (pls) => {
      if (pls.id === playlistId) {
        refetch()
      }
    },
    [playlistId, refetch],
  )

  const reorder = useCallback(
    (playlistId, id, newPos) => {
      dataProvider
        .update('playlistTrack', {
          id,
          data: { insert_before: newPos },
          filter: { playlist_id: playlistId },
        })
        .then(() => {
          refetch()
        })
        .catch(() => {
          notify('ra.page.error', { type: 'warning' })
        })
    },
    [dataProvider, notify, refetch],
  )

  const handleDragEnd = useCallback(
    (from, to) => {
      const toId = ids[to]
      const fromId = ids[from]
      reorder(playlistId, fromId, toId)
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
      <ExpandInfoDialog content={<SongInfo />} />
      {React.cloneElement(props.pagination, listContext)}
    </>
  )
}

const SanitizedPlaylistSongs = (props) => {
  const { isPending } = useListContext()
  return (
    <>
      {!isPending && (
        <PlaylistSongs
          playlistId={props.id}
          actions={props.actions}
          pagination={props.pagination}
          {...props}
        />
      )}
    </>
  )
}

export default SanitizedPlaylistSongs
