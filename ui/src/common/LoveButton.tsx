import React, { useCallback } from 'react'
import FavoriteIcon from '@mui/icons-material/Favorite'
import FavoriteBorderIcon from '@mui/icons-material/FavoriteBorder'
import IconButton from '@mui/material/IconButton'
import { useToggleLove } from './useToggleLove'
import { useRecordContext, useTranslate } from 'react-admin'
import config from '../config'
import { isDateSet } from '../utils/validations'

import type { Identifier } from 'react-admin'

type LoveRecord = {
  id?: Identifier
  missing?: boolean
  starredAt?: string
}

export const LoveButton = ({
  resource,
  color = 'inherit',
  visible = true,
  size = 'small',
  component: Button = IconButton,
  addLabel = true,
  disabled = false,
  className,
  record: recordProp,
  sx,
  ...rest
}: {
  resource: string
  color?: string
  visible?: boolean
  size?: 'small' | 'medium' | 'large'
  component?: typeof IconButton
  addLabel?: boolean
  disabled?: boolean
  className?: string
  record?: LoveRecord
  sx?: unknown
  [key: string]: unknown
}) => {
  const record = (useRecordContext<LoveRecord>({ record: recordProp }) ||
    {}) as LoveRecord
  const translate = useTranslate()
  const [toggleLove, loading, loved] = useToggleLove(resource, record)

  const handleToggleLove = useCallback(
    (e) => {
      e.preventDefault()
      toggleLove()
      e.stopPropagation()
    },
    [toggleLove],
  )

  if (!config.enableFavourites) {
    return <></>
  }
  return (
    <Button
      onClick={handleToggleLove}
      size={'small'}
      disabled={disabled || loading || record.missing}
      className={className}
      sx={[
        {
          color,
          visibility:
            visible === false ? 'hidden' : loved ? 'visible' : 'inherit',
        },
        ...(Array.isArray(sx) ? sx : [sx]),
      ]}
      aria-label={translate('message.toggle_love')}
      aria-pressed={loved}
      title={
        isDateSet(record.starredAt)
          ? new Date(record.starredAt).toLocaleString()
          : undefined
      }
      {...rest}
    >
      {loved ? (
        <FavoriteIcon fontSize={size as 'small' | 'medium' | 'large'} />
      ) : (
        <FavoriteBorderIcon fontSize={size as 'small' | 'medium' | 'large'} />
      )}
    </Button>
  )
}
