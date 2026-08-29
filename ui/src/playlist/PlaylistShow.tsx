import React from 'react'
import {
  ReferenceManyField,
  ShowContextProvider,
  useShowContext,
  useShowController,
  Title as RaTitle,
} from 'react-admin'
import { styled } from '@mui/material/styles'
import PlaylistDetails from './PlaylistDetails'
import PlaylistSongs from './PlaylistSongs'
import PlaylistActions from './PlaylistActions'
import {
  Pagination,
  Title,
  canChangeTracks,
  getStoredPerPage,
  useResourceRefresh,
} from '../common'
import { componentStyleOverride } from '../themes/componentStyleOverride'
import type { Identifier } from 'react-admin'
import type { PlaylistRecord } from '../types/records'

const playlistTrackPerPageOptions = [100, 250, 500]

const FullWidthPlaylistActions = styled(PlaylistActions)(({ theme }) => ({
  width: '100%',
  ...componentStyleOverride(theme, 'NDPlaylistShow', 'playlistActions'),
}))

type PlaylistShowLayoutProps = {
  id?: Identifier
}

const PlaylistShowLayout = (props: PlaylistShowLayoutProps) => {
  const context = useShowContext()
  const { record } = context
  useResourceRefresh('song')

  return (
    <>
      {record && <RaTitle title={<Title subTitle={record.name} />} />}
      {record && <PlaylistDetails record={record as PlaylistRecord} />}
      {record && (
        <ReferenceManyField
          reference="playlistTrack"
          target="playlist_id"
          sort={{ field: 'id', order: 'ASC' }}
          perPage={getStoredPerPage(
            'playlistTrack',
            playlistTrackPerPageOptions,
          )}
          filter={{ playlist_id: props.id }}
        >
          <PlaylistSongs
            {...props}
            readOnly={!canChangeTracks(record)}
            actions={
              <FullWidthPlaylistActions record={record as PlaylistRecord} />
            }
            resource={'playlistTrack'}
            exporter={false}
            pagination={
              <Pagination rowsPerPageOptions={playlistTrackPerPageOptions} />
            }
          />
        </ReferenceManyField>
      )}
    </>
  )
}

const PlaylistShow = (props: Record<string, unknown>) => {
  const controllerProps = useShowController(props)
  return (
    <ShowContextProvider value={controllerProps}>
      <PlaylistShowLayout {...props} />
    </ShowContextProvider>
  )
}

export default PlaylistShow
