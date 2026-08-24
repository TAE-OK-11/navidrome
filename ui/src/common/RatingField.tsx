// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useCallback } from 'react'
import PropTypes from 'prop-types'
import Rating from '@mui/material/Rating'
import { isDateSet } from '../utils/validations'
import StarBorderIcon from '@mui/icons-material/StarBorder'
import { useRating } from './useRating'
import { useRecordContext } from 'react-admin'

export const RatingField = ({
  resource,
  visible = true,
  className,
  size = 'small',
  color = 'inherit',
  ...rest
}) => {
  const record = useRecordContext(rest) || {}
  const [rate, rating, loading] = useRating(resource, record)

  const stopPropagation = (e) => {
    e.stopPropagation()
  }

  const handleRating = useCallback(
    (e, val) => {
      const targetId = record.mediaFileId || record.id
      rate(val ?? 0, targetId)
    },
    [rate, record.mediaFileId, record.id],
  )

  return (
    <span
      onClick={(e) => stopPropagation(e)}
      title={
        isDateSet(record.ratedAt)
          ? new Date(record.ratedAt).toLocaleString()
          : undefined
      }
    >
      <Rating
        name={record.mediaFileId || record.id}
        className={className}
        sx={[
          {
            color,
            visibility: visible === false ? 'hidden' : 'inherit',
          },
          rating > 0
            ? { visibility: 'visible !important' }
            : { visibility: 'hidden' },
        ]}
        value={rating}
        size={size}
        disabled={record?.missing || loading}
        emptyIcon={<StarBorderIcon fontSize="inherit" />}
        onChange={(e, newValue) => handleRating(e, newValue)}
      />
    </span>
  )
}
RatingField.propTypes = {
  resource: PropTypes.string.isRequired,
  record: PropTypes.object,
  visible: PropTypes.bool,
  size: PropTypes.string,
}
