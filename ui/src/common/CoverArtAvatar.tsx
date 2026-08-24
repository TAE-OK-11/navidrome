// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { useRecordContext } from 'react-admin'
import { Avatar } from '@mui/material'
import config from '../config'
import subsonic from '../subsonic'
import { useImageUrl } from './useImageUrl'

export const CoverArtAvatar = ({
  record: recordProp,
  variant = 'circular',
}) => {
  const recordContext = useRecordContext()
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
