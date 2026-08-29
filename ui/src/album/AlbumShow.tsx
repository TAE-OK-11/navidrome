import React from 'react'
import {
  ReferenceManyField,
  ShowContextProvider,
  useShowContext,
  useShowController,
  Title as RaTitle,
} from 'react-admin'
import { styled } from '@mui/material/styles'
import AlbumSongs from './AlbumSongs'
import AlbumDetails from './AlbumDetails'
import AlbumActions from './AlbumActions'
import { useResourceRefresh, useScrollRestoration, Title } from '../common'
import { componentStyleOverride } from '../themes/componentStyleOverride'
import type { AlbumRecord } from '../types/records'

const FullWidthAlbumActions = styled(AlbumActions)(({ theme }) => ({
  width: '100%',
  ...componentStyleOverride(theme, 'NDAlbumShow', 'albumActions'),
}))

const AlbumShowLayout = () => {
  const context = useShowContext()
  const { record } = context
  useResourceRefresh('album', 'song')
  useScrollRestoration(!!record?.id)

  return (
    <>
      {record && <RaTitle title={<Title subTitle={record.name} />} />}
      {record && <AlbumDetails record={record as AlbumRecord} />}
      {record && (
        <ReferenceManyField
          reference="song"
          target="album_id"
          sort={{ field: 'album', order: 'ASC' }}
          perPage={-1}
          pagination={null}
        >
          <AlbumSongs
            resource={'song'}
            exporter={false}
            album={record}
            actions={<FullWidthAlbumActions record={record as AlbumRecord} />}
          />
        </ReferenceManyField>
      )}
    </>
  )
}

const AlbumShow = (props: Record<string, unknown>) => {
  const controllerProps = useShowController(props)
  return (
    <ShowContextProvider value={controllerProps}>
      <AlbumShowLayout />
    </ShowContextProvider>
  )
}

export default AlbumShow
