// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
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

const playlistTrackPerPageOptions = [100, 250, 500]

const FullWidthPlaylistActions = styled(PlaylistActions)(({ theme }) => ({
  width: '100%',
  ...componentStyleOverride(theme, 'NDPlaylistShow', 'playlistActions'),
}))

const PlaylistShowLayout = (props) => {
  const context = useShowContext(props)
  const { record } = context
  useResourceRefresh('song')

  return (
    <>
      {record && <RaTitle title={<Title subTitle={record.name} />} />}
      {record && <PlaylistDetails {...context} />}
      {record && (
        <ReferenceManyField
          {...context}
          addLabel={false}
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
            title={<Title subTitle={record.name} />}
            actions={<FullWidthPlaylistActions record={record} />}
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

const PlaylistShow = (props) => {
  const controllerProps = useShowController(props)
  return (
    <ShowContextProvider value={controllerProps}>
      <PlaylistShowLayout {...props} {...controllerProps} />
    </ShowContextProvider>
  )
}

export default PlaylistShow
