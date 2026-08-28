import React from 'react'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import { IconButton } from '@mui/material'
import type { IconButtonProps } from '@mui/material'
import { useDispatch } from 'react-redux'
import { useDataProvider, useRecordContext } from 'react-admin'
import { playTracks } from '../actions'

type PlayRecord = {
  id: string | number
  discNumber?: number
  [key: string]: unknown
}

type PlayButtonProps = {
  record?: PlayRecord
  size?: IconButtonProps['size']
  className?: string
}

export const PlayButton = ({
  record: recordOverride,
  size = 'small',
  className,
}: PlayButtonProps) => {
  const record = useRecordContext<PlayRecord>({ record: recordOverride })
  const extractSongsData = (response: { data: PlayRecord[] }) => {
    const data: Record<string, PlayRecord> = {}
    const ids: Array<string | number> = []
    for (const song of response.data) {
      data[String(song.id)] = song
      ids.push(song.id)
    }
    return { data, ids }
  }
  const dataProvider = useDataProvider()
  const dispatch = useDispatch()
  const playAlbum = (album: PlayRecord) => {
    dataProvider
      .getList('song', {
        pagination: { page: 1, perPage: -1 },
        sort: { field: 'album', order: 'ASC' },
        filter: {
          album_id: album.id,
          disc_number: album.discNumber,
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
        if (record) playAlbum(record)
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
