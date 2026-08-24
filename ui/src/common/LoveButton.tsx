// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useCallback } from 'react'
import PropTypes from 'prop-types'
import FavoriteIcon from '@mui/icons-material/Favorite'
import FavoriteBorderIcon from '@mui/icons-material/FavoriteBorder'
import IconButton from '@mui/material/IconButton'
import makeStyles from '../themes/makeStyles'
import clsx from 'clsx'
import { useToggleLove } from './useToggleLove'
import { useRecordContext, useTranslate } from 'react-admin'
import config from '../config'
import { isDateSet } from '../utils/validations'

const useStyles = makeStyles(
  {
    love: {
      color: (props) => props.color,
      visibility: (props) =>
        props.visible === false
          ? 'hidden'
          : props.loved
            ? 'visible'
            : 'inherit',
    },
  },
  { name: 'NDLoveButton' },
)

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
  ...rest
}) => {
  const record = useRecordContext({ record: recordProp }) || {}
  const translate = useTranslate()
  const [toggleLove, loading, loved] = useToggleLove(resource, record)
  const classes = useStyles({ color, visible, loved })

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
      className={clsx(classes.love, className)}
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
        <FavoriteIcon fontSize={size} />
      ) : (
        <FavoriteBorderIcon fontSize={size} />
      )}
    </Button>
  )
}

LoveButton.propTypes = {
  resource: PropTypes.string.isRequired,
  record: PropTypes.object,
  visible: PropTypes.bool,
  color: PropTypes.string,
  size: PropTypes.string,
  component: PropTypes.object,
  disabled: PropTypes.bool,
}
