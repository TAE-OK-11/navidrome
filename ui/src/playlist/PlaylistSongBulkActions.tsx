// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { Fragment, useEffect } from 'react'
import {
  BulkDeleteButton,
  useUnselectAll,
  ResourceContextProvider,
} from 'react-admin'
import { MdOutlinePlaylistRemove } from 'react-icons/md'

// Replace original resource with "fake" one for removing tracks from playlist
const PlaylistSongBulkActions = ({
  playlistId,
  resource,
  onUnselectItems,
  ...rest
}) => {
  const unselectAll = useUnselectAll()
  useEffect(() => {
    unselectAll('playlistTrack')
  }, [unselectAll])

  const mappedResource = `playlist/${playlistId}/tracks`
  return (
    <ResourceContextProvider value={mappedResource}>
      <Fragment>
        <BulkDeleteButton
          {...rest}
          label={'ra.action.remove'}
          icon={<MdOutlinePlaylistRemove />}
          resource={mappedResource}
          onClick={onUnselectItems}
        />
      </Fragment>
    </ResourceContextProvider>
  )
}

export default PlaylistSongBulkActions
