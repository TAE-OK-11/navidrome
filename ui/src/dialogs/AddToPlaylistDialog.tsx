import React, { useState } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import {
  useDataProvider,
  useNotify,
  useRefresh,
  useTranslate,
} from 'react-admin'
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
} from '@mui/material'
import {
  closeAddToPlaylist,
  closeDuplicateSongDialog,
  openDuplicateSongWarning,
} from '../actions'
import { SelectPlaylistInput } from './SelectPlaylistInput'
import DuplicateSongDialog from './DuplicateSongDialog'
import { httpClient } from '../dataProvider'
import { REST_URL } from '../consts'

import type { NavidromeRootState } from '../types/redux'
import type { PlaylistSelection } from '../types/records'
import type { Identifier } from 'react-admin'

export const AddToPlaylistDialog = () => {
  const {
    open,
    selectedIds = [],
    onSuccess,
    duplicateSong,
    duplicateIds = [],
  } = useSelector((state: NavidromeRootState) => state.addToPlaylistDialog)
  const dispatch = useDispatch()
  const translate = useTranslate()
  const notify = useNotify()
  const refresh = useRefresh()
  const [value, setValue] = useState<PlaylistSelection[]>([])
  const [check, setCheck] = useState(false)
  const dataProvider = useDataProvider()
  const createAndAddToPlaylist = (playlistObject: PlaylistSelection) => {
    dataProvider
      .create('playlist', {
        data: { name: playlistObject.name },
      })
      .then((res) => {
        addToPlaylist(res.data.id as Identifier)
      })
      .catch((error) => notify(`Error: ${error.message}`, { type: 'warning' }))
  }

  const addToPlaylist = (
    playlistId: Identifier,
    distinctIds?: Identifier[],
  ) => {
    const trackIds = Array.isArray(distinctIds) ? distinctIds : selectedIds
    if (trackIds.length) {
      dataProvider
        .create('playlistTrack', {
          data: { ids: trackIds },
          filter: { playlist_id: playlistId },
        })
        .then(() => {
          const len = trackIds.length
          notify('message.songsAddedToPlaylist', {
            messageArgs: { smart_count: len },
          })
          onSuccess?.(value, len)
          refresh()
        })
        .catch(() => {
          notify('ra.page.error', { type: 'warning' })
        })
    } else {
      notify('message.songsAddedToPlaylist', {
        messageArgs: { smart_count: 0 },
      })
    }
  }

  const checkDuplicateSong = (playlistObject) => {
    httpClient(`${REST_URL}/playlist/${playlistObject.id}/tracks`)
      .then((res) => {
        const tracks = res.json
        if (tracks) {
          const dupSng = tracks.filter((song) =>
            selectedIds.some((id) => id === song.mediaFileId),
          )

          if (dupSng.length) {
            const dupIds = dupSng.map((song) => song.mediaFileId)
            dispatch(openDuplicateSongWarning(dupIds))
          }
        }
        setCheck(true)
      })
      .catch(() => {
        notify('ra.page.error', { type: 'warning' })
      })
  }

  const handleSubmit = (e) => {
    value.forEach((playlistObject) => {
      if (playlistObject.id) {
        addToPlaylist(playlistObject.id, playlistObject.distinctIds)
      } else {
        createAndAddToPlaylist(playlistObject)
      }
    })
    setCheck(false)
    setValue([])
    dispatch(closeAddToPlaylist())
    e.stopPropagation()
  }

  const handleClickClose = (e) => {
    setCheck(false)
    setValue([])
    dispatch(closeAddToPlaylist())
    e.stopPropagation()
  }

  const handleChange = (pls: PlaylistSelection[]) => {
    if (!value.length || pls.length > value.length) {
      const newlyAdded = pls.slice(-1).pop()
      if (newlyAdded?.id) {
        setCheck(false)
        checkDuplicateSong(newlyAdded)
      } else setCheck(true)
    } else if (pls.length === 0) setCheck(false)
    setValue(pls)
  }

  const handleDuplicateClose = () => {
    dispatch(closeDuplicateSongDialog())
  }
  const handleDuplicateSubmit = () => {
    dispatch(closeDuplicateSongDialog())
  }
  const handleSkip = () => {
    const distinctSongs = selectedIds.filter(
      (id) => duplicateIds.indexOf(id) < 0,
    )
    const lastSelection = value.slice(-1).pop()
    if (lastSelection) {
      lastSelection.distinctIds = distinctSongs
    }
    dispatch(closeDuplicateSongDialog())
  }

  return (
    <>
      <Dialog
        open={open}
        onClose={handleClickClose}
        aria-labelledby="form-dialog-new-playlist"
        fullWidth={true}
        maxWidth={'sm'}
        slotProps={{
          paper: { sx: { height: '26em', maxHeight: '26em' } },
        }}
      >
        <DialogTitle id="form-dialog-new-playlist">
          {translate('resources.playlist.actions.selectPlaylist')}
        </DialogTitle>
        <DialogContent
          sx={{ height: '17.5em', overflowY: 'auto', py: '0.5em' }}
        >
          <SelectPlaylistInput onChange={handleChange} />
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClickClose} color="primary">
            {translate('ra.action.cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            color="primary"
            disabled={!check}
            data-testid="playlist-add"
          >
            {translate('ra.action.add')}
          </Button>
        </DialogActions>
      </Dialog>
      <DuplicateSongDialog
        open={duplicateSong}
        handleClickClose={handleDuplicateClose}
        handleSubmit={handleDuplicateSubmit}
        handleSkip={handleSkip}
      />
    </>
  )
}
