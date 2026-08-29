import React, { cloneElement } from 'react'
import { sanitizeListRestProps, TopToolbar } from 'react-admin'
import { useMediaQuery } from '@mui/material'
import { ToggleFieldsMenu } from '../common'

type ArtistListActionsProps = {
  className?: string
  filters?: React.ReactElement
  resource?: string
  showFilter?: boolean
  displayedFilters?: unknown
  filterValues?: unknown
}

const ArtistListActions = ({
  className,
  filters,
  resource,
  showFilter,
  displayedFilters,
  filterValues,
  ...rest
}: ArtistListActionsProps) => {
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))

  return (
    <TopToolbar className={className} {...sanitizeListRestProps(rest)}>
      {filters &&
        cloneElement(filters, {
          resource,
          showFilter,
          displayedFilters,
          filterValues,
          context: 'button',
        } as Record<string, unknown>)}
      {isNotSmall && <ToggleFieldsMenu resource="artist" />}
    </TopToolbar>
  )
}

export default ArtistListActions
