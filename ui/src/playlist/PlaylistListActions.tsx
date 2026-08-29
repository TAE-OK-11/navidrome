import React, { cloneElement } from 'react'
import {
  sanitizeListRestProps,
  TopToolbar,
  CreateButton,
  useTranslate,
} from 'react-admin'
import { useMediaQuery } from '@mui/material'
import { ToggleFieldsMenu } from '../common'

type PlaylistListActionsProps = {
  className?: string
  filters?: React.ReactElement
}

const PlaylistListActions = ({
  className,
  ...rest
}: PlaylistListActionsProps) => {
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))
  const translate = useTranslate()

  return (
    <TopToolbar className={className} {...sanitizeListRestProps(rest)}>
      {rest.filters &&
        cloneElement(rest.filters, { context: 'button' } as Record<
          string,
          unknown
        >)}
      <CreateButton>{translate('ra.action.create')}</CreateButton>
      {isNotSmall && <ToggleFieldsMenu resource="playlist" />}
    </TopToolbar>
  )
}

export default PlaylistListActions
