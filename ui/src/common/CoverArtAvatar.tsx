import { useRecordContext } from 'react-admin'
import { Avatar } from '@mui/material'
import type { AvatarProps } from '@mui/material'
import config from '../config'
import subsonic from '../subsonic'
import { useImageUrl } from './useImageUrl'

type CoverArtRecord = {
  id: string | number
  name?: string
  [key: string]: unknown
}

type CoverArtAvatarProps = {
  record?: CoverArtRecord
  variant?: AvatarProps['variant']
  label?: string
  sortable?: boolean
  source?: string
}

export const CoverArtAvatar = ({
  record: recordProp,
  variant = 'circular',
}: CoverArtAvatarProps) => {
  const recordContext = useRecordContext<CoverArtRecord>()
  const record = recordProp || recordContext
  const square = variant !== 'circular'
  const url = record
    ? subsonic.getCoverArtUrl(record, config.uiCoverArtSize, square)
    : null
  const { imgUrl } = useImageUrl(url)
  if (!record) return null
  return (
    <Avatar
      src={imgUrl || undefined}
      variant={variant}
      sx={{
        width: 55,
        height: 55,
        ...(square && { borderRadius: 1 }),
        ...(!imgUrl && { backgroundColor: 'transparent' }),
      }}
      alt={record.name}
    >
      {/* Empty child prevents default person icon while loading */}
      {!imgUrl && <span />}
    </Avatar>
  )
}

CoverArtAvatar.defaultProps = { label: '', sortable: false }
