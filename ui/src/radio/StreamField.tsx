import { Button } from '@mui/material'
import React, { useCallback } from 'react'
import { useRecordContext } from 'react-admin'
import { useDispatch } from 'react-redux'
import { setTrack } from '../actions'
import { songFromRadio } from './helper'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'

export const StreamField = (props) => {
  const record = useRecordContext(props)
  const dispatch = useDispatch()

  const playTrack = useCallback(
    async (evt) => {
      evt.stopPropagation()
      evt.preventDefault()
      dispatch(setTrack(await songFromRadio(record)))
    },
    [dispatch, record],
  )

  return (
    <Button
      sx={{ py: '5px', px: 0, textTransform: 'none', mr: 1.5 }}
      onClick={playTrack}
    >
      <PlayArrowIcon />
    </Button>
  )
}

StreamField.defaultProps = {
  addLabel: true,
}
