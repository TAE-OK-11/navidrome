import { useMemo } from 'react'
import Chip from '@mui/material/Chip'
import config from '../config'
import { calculateGain } from '../utils/calculateReplayGain'
import { useRecordContext } from 'react-admin'

const llFormats = new Set(config.losslessFormats.split(','))
const placeholder = 'N/A'

type QualityRecord = {
  suffix?: string
  bitRate?: number
  rgAlbumGain?: number
  rgAlbumPeak?: number
  rgTrackGain?: number
  rgTrackPeak?: number
}

type QualityInfoProps = {
  record?: QualityRecord
  size?: 'small' | 'medium'
  gainMode?: string
  preAmp?: number
  className?: string
  transcodeStream?: { codec?: string; audioBitrate?: number }
  isDirectPlay?: boolean
  source?: string
  sortable?: boolean
}

export const QualityInfo = ({
  record: recordOverride,
  size = 'small',
  gainMode = 'none',
  preAmp,
  className,
  transcodeStream,
  isDirectPlay,
}: QualityInfoProps) => {
  const record =
    useRecordContext<QualityRecord>({ record: recordOverride }) || {}
  let { suffix, bitRate, rgAlbumGain, rgAlbumPeak, rgTrackGain, rgTrackPeak } =
    record
  let info = placeholder

  if (suffix) {
    suffix = suffix.toUpperCase()
    info = suffix
    if (!llFormats.has(suffix) && typeof bitRate === 'number' && bitRate > 0) {
      info += ' ' + bitRate
    }
  }

  // Show transcode target when transcoding (not direct play)
  if (transcodeStream && !isDirectPlay) {
    const targetCodec = (transcodeStream.codec || '').toUpperCase()
    const targetBitrate = transcodeStream.audioBitrate
      ? Math.round(transcodeStream.audioBitrate / 1000)
      : 0
    let targetInfo = targetCodec
    if (targetBitrate > 0) {
      targetInfo += ' ' + targetBitrate
    }
    const sourceSuffix = suffix || placeholder
    info = `${sourceSuffix} → ${targetInfo}`
  }

  const extra = useMemo(() => {
    if (gainMode !== 'none') {
      const gainValue = calculateGain(
        { gainMode, preAmp },
        { rgAlbumGain, rgAlbumPeak, rgTrackGain, rgTrackPeak },
      )
      // convert normalized gain (after peak) back to dB
      const toDb = (Math.log10(gainValue) * 20).toFixed(2)
      return ` (${toDb} dB)`
    }

    return ''
  }, [gainMode, preAmp, rgAlbumGain, rgAlbumPeak, rgTrackGain, rgTrackPeak])

  return (
    <Chip
      className={className}
      sx={{ transform: 'scale(0.8)' }}
      variant="outlined"
      size={size}
      label={`${info}${extra}`}
    />
  )
}
