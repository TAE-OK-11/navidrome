import React from 'react'
import PropTypes from 'prop-types'
import { Link, useRecordContext } from 'react-admin'
import { useDispatch } from 'react-redux'
import { closeExtendedInfoDialog } from '../actions'

export const AlbumLinkField = (props) => {
  const dispatch = useDispatch()
  const record = useRecordContext(props)

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

AlbumLinkField.propTypes = {
  sortBy: PropTypes.string,
  sortByOrder: PropTypes.oneOf(['ASC', 'DESC']),
}

AlbumLinkField.defaultProps = {
  addLabel: true,
}
