import React, { Fragment, useEffect } from 'react'
import { useUnselectAll } from 'react-admin'
import { addTracks, playNext, playTracks } from '../actions'
import { RiPlayList2Fill, RiPlayListAddFill } from 'react-icons/ri'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import { BatchPlayButton } from './index'
import { AddToPlaylistButton } from './AddToPlaylistButton'
import { BatchShareButton } from './BatchShareButton'
import config from '../config'

const buttonSx = {
  color: (theme) => (theme.palette.mode === 'dark' ? 'white' : undefined),
}

export const SongBulkActions = (props) => {
  const unselectAll = useUnselectAll()
  useEffect(() => {
    unselectAll(props.resource)
  }, [unselectAll, props.resource])
  return (
    <Fragment>
      <BatchPlayButton
        {...props}
        action={playTracks}
        label={'resources.song.actions.playNow'}
        icon={<PlayArrowIcon />}
        sx={buttonSx}
      />
      <BatchPlayButton
        {...props}
        action={playNext}
        label={'resources.song.actions.playNext'}
        icon={<RiPlayList2Fill />}
        sx={buttonSx}
      />
      <BatchPlayButton
        {...props}
        action={addTracks}
        label={'resources.song.actions.addToQueue'}
        icon={<RiPlayListAddFill />}
        sx={buttonSx}
      />
      {config.enableSharing && <BatchShareButton {...props} sx={buttonSx} />}
      <AddToPlaylistButton {...props} sx={buttonSx} />
    </Fragment>
  )
}
