import React, { Fragment, useEffect } from 'react'
import {
  BulkDeleteButton,
  useUnselectAll,
  ResourceContextProvider,
} from 'react-admin'
import { MdOutlinePlaylistRemove } from 'react-icons/md'
import type { Identifier } from 'react-admin'

type PlaylistSongBulkActionsProps = {
  playlistId?: Identifier
  resource?: string
  onUnselectItems?: () => void
  readOnly?: boolean
}

const PlaylistSongBulkActions = ({
  playlistId,
  resource,
  onUnselectItems,
  ...rest
}: PlaylistSongBulkActionsProps) => {
  const unselectAll = useUnselectAll('playlistTrack')
  useEffect(() => {
    unselectAll()
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
