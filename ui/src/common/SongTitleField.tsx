import React from 'react'
import { useSelector } from 'react-redux'
import { FunctionField, useRecordContext } from 'react-admin'
import Box from '@mui/material/Box'
import { useTheme } from '@mui/material/styles'
import PlayingLight from '../icons/playing-light.gif'
import PlayingDark from '../icons/playing-dark.gif'
import PausedLight from '../icons/paused-light.png'
import PausedDark from '../icons/paused-dark.png'
import type { AppState } from '../types/redux'
import type { SongRecord } from '../types/records'

type SongTitleFieldProps = {
  showTrackNumbers?: boolean
  record?: SongRecord
  source?: string
  sortable?: boolean
  label?: React.ReactNode
}

export const SongTitleField = ({
  showTrackNumbers = false,
  record: recordOverride,
  ...props
}: SongTitleFieldProps) => {
  const record = useRecordContext<SongRecord>({ record: recordOverride })
  const theme = useTheme()
  const currentTrack = useSelector(
    (state: AppState) => state?.player?.current || {},
  )
  if (!record) return null
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
          <Box component="span" sx={{ opacity: 0.5 }}>
            {' (' + subtitle + ')'}
          </Box>
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
      <Box
        component="img"
        src={icon}
        sx={{
          width: 32,
          height: 32,
          verticalAlign: 'text-top',
          ml: '-8px',
          mt: '-7px',
          pr: '3px',
        }}
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
        sx={{ verticalAlign: 'text-top' }}
      />
    </>
  )
}
