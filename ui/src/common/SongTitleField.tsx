// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import makeStyles from '../themes/makeStyles'
import React from 'react'
import PropTypes from 'prop-types'
import { useSelector } from 'react-redux'
import { FunctionField, useRecordContext } from 'react-admin'
import { useTheme } from '@mui/material/styles'
import PlayingLight from '../icons/playing-light.gif'
import PlayingDark from '../icons/playing-dark.gif'
import PausedLight from '../icons/paused-light.png'
import PausedDark from '../icons/paused-dark.png'

const useStyles = makeStyles({
  icon: {
    width: '32px',
    height: '32px',
    verticalAlign: 'text-top',
    marginLeft: '-8px',
    marginTop: '-7px',
    paddingRight: '3px',
  },
  text: {
    verticalAlign: 'text-top',
  },
  subtitle: {
    opacity: 0.5,
  },
})

export const SongTitleField = ({
  showTrackNumbers = false,
  record: recordOverride,
  ...props
}) => {
  const record = useRecordContext({ record: recordOverride }) || {}
  const theme = useTheme()
  const classes = useStyles()
  const currentTrack = useSelector((state) => state?.player?.current || {})
  const currentId = currentTrack.trackId
  const paused = currentTrack.paused
  const isCurrent =
    currentId && (currentId === record.id || currentId === record.mediaFileId)

  const subtitle = record?.tags?.['subtitle']

  const trackName = (r) => {
    const name = r.title
    if (r.trackNumber && showTrackNumbers) {
      return r.trackNumber.toString().padStart(2, '0') + ' ' + name
    }
    if (subtitle) {
      return (
        <>
          {name}
          <span className={classes.subtitle}>{' (' + subtitle + ')'}</span>
        </>
      )
    }
    return name
  }

  const Icon = () => {
    let icon
    if (paused) {
      icon = theme.palette.mode === 'light' ? PausedLight : PausedDark
    } else {
      icon = theme.palette.mode === 'light' ? PlayingLight : PlayingDark
    }
    return (
      <img
        src={icon}
        className={classes.icon}
        alt={paused ? 'paused' : 'playing'}
      />
    )
  }

  return (
    <>
      {isCurrent && <Icon />}
      <FunctionField
        {...props}
        source="title"
        render={trackName}
        className={classes.text}
      />
    </>
  )
}

SongTitleField.propTypes = {
  record: PropTypes.object,
  showTrackNumbers: PropTypes.bool,
}
