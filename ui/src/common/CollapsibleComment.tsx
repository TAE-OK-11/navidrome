import { useCallback, useMemo, useState } from 'react'
import { Typography, Collapse } from '@mui/material'
import AnchorMe from './Linkify'

type CollapsibleCommentProps = {
  record: {
    id: string
    comment?: string
  }
}

export const CollapsibleComment = ({ record }: CollapsibleCommentProps) => {
  const [expanded, setExpanded] = useState(false)

  const lines = useMemo(
    () => record.comment?.split('\n') || [],
    [record.comment],
  )
  const formatted = useMemo(() => {
    return lines.map((line, idx) => (
      <span key={record.id + '-comment-' + idx}>
        <AnchorMe text={line} />
        <br />
      </span>
    ))
  }, [lines, record.id])

  const handleExpandClick = useCallback(() => {
    setExpanded((current) => !current)
  }, [])

  if (lines.length === 0) {
    return null
  }

  return (
    <Collapse
      collapsedSize={'2em'}
      in={expanded}
      timeout={'auto'}
      sx={{
        display: 'inline-block',
        mt: '1em',
        float: 'left',
        wordBreak: 'break-word',
        cursor: lines.length > 1 ? 'pointer' : 'inherit',
      }}
    >
      <Typography variant={'h6'} onClick={handleExpandClick}>
        {formatted}
      </Typography>
    </Collapse>
  )
}
