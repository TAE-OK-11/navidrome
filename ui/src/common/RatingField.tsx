import React, { useCallback } from 'react'
import Rating from '@mui/material/Rating'
import type { SxProps, Theme } from '@mui/material/styles'
import { isDateSet } from '../utils/validations'
import StarBorderIcon from '@mui/icons-material/StarBorder'
import { useRating } from './useRating'
import { useRecordContext } from 'react-admin'
import clsx from 'clsx'
import type { Identifier } from 'react-admin'

type RatingRecord = {
  id?: Identifier
  mediaFileId?: Identifier
  ratedAt?: string
  missing?: boolean
}

type RatingFieldProps = {
  resource: string
  source?: string
  sortable?: boolean
  sortByOrder?: string
  label?: React.ReactNode
  visible?: boolean
  className?: string
  size?: 'small' | 'medium' | 'large'
  color?: string
  sx?: SxProps<Theme>
  record?: RatingRecord
}

export const RatingField = ({
  resource,
  visible = true,
  className,
  size = 'small',
  color = 'inherit',
  sx,
  ...rest
}: RatingFieldProps) => {
  const record = useRecordContext<RatingRecord>(rest)
  const [setSongRating, currentRating, isLoading] = useRating(
    resource,
    record ?? {},
  )

  if (!record) {
    return null
  }

  const stopPropagation = (e: React.MouseEvent) => {
    e.stopPropagation()
  }

  const handleRating = useCallback(
    (_e: React.SyntheticEvent, val: number | null) => {
      const targetId = record.mediaFileId || record.id
      if (targetId !== undefined) {
        void setSongRating(val ?? 0, targetId)
      }
    },
    [setSongRating, record.mediaFileId, record.id],
  )

  return (
    <span
      onClick={(e) => stopPropagation(e)}
      title={
        record.ratedAt && isDateSet(record.ratedAt)
          ? new Date(record.ratedAt).toLocaleString()
          : undefined
      }
    >
      <Rating
        name={String(record.mediaFileId || record.id)}
        className={clsx('nd-rating-field', className)}
        sx={[
          {
            color,
            visibility: visible === false ? 'hidden' : 'inherit',
          },
          currentRating > 0
            ? { visibility: 'visible !important' }
            : { visibility: 'hidden' },
          ...(Array.isArray(sx) ? sx : [sx]),
        ]}
        value={currentRating}
        size={size}
        disabled={Boolean(record.missing) || isLoading}
        emptyIcon={<StarBorderIcon fontSize="inherit" />}
        onChange={handleRating}
      />
    </span>
  )
}
