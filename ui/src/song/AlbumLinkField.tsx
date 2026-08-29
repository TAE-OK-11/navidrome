import React from 'react'
import { Link, useRecordContext } from 'react-admin'
import { useDispatch } from 'react-redux'
import { closeExtendedInfoDialog } from '../actions'
import type { SongRecord } from '../types/records'

type AlbumLinkFieldProps = {
  record?: SongRecord
  source?: string
  sortable?: boolean
  sortByOrder?: string
  label?: React.ReactNode
}

export const AlbumLinkField = (props: AlbumLinkFieldProps) => {
  const dispatch = useDispatch()
  const record = useRecordContext<SongRecord>(props)

  if (!record?.albumId) return null

  return (
    <Link
      to={`/album/${record.albumId}/show`}
      onClick={(e) => {
        e.stopPropagation()
        dispatch(closeExtendedInfoDialog())
      }}
    >
      {record.album}
    </Link>
  )
}

AlbumLinkField.defaultProps = {
  addLabel: true,
}
