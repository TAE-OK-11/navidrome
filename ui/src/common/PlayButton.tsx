// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import PropTypes from 'prop-types'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import { IconButton } from '@mui/material'
import { useDispatch } from 'react-redux'
import { useDataProvider, useRecordContext } from 'react-admin'
import { playTracks } from '../actions'

export const PlayButton = ({
  record: recordOverride,
  size = 'small',
  className,
}) => {
  const record = useRecordContext({ record: recordOverride })
  const extractSongsData = (response) => {
    const data = {}
    const ids = []
    for (const song of response.data) {
      data[song.id] = song
      ids.push(song.id)
    }
    return { data, ids }
  }
  const dataProvider = useDataProvider()
  const dispatch = useDispatch()
  const playAlbum = (record) => {
    dataProvider
      .getList('song', {
        pagination: { page: 1, perPage: -1 },
        sort: { field: 'album', order: 'ASC' },
        filter: {
          album_id: record.id,
          disc_number: record.discNumber,
        },
      })
      .then((response) => {
        const { data, ids } = extractSongsData(response)
        dispatch(playTracks(data, ids))
      })
  }

  return (
    <IconButton
      onClick={(e) => {
        e.stopPropagation()
        e.preventDefault()
        playAlbum(record)
      }}
      aria-label="play"
      className={className}
      size={size}
      disabled={!record?.id}
    >
      <PlayArrowIcon fontSize={size} />
    </IconButton>
  )
}

PlayButton.propTypes = {
  record: PropTypes.object.isRequired,
  size: PropTypes.string,
  className: PropTypes.string,
}
