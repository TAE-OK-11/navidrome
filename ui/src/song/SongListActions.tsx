import React, { cloneElement } from 'react'
import { sanitizeListRestProps, TopToolbar } from 'react-admin'
import { useMediaQuery } from '@mui/material'
import { ShuffleAllButton, ToggleFieldsMenu } from '../common'

type SongListActionsProps = {
  currentSort?: unknown
  className?: string
  resource?: string
  filters?: React.ReactElement
  displayedFilters?: unknown
  filterValues?: unknown
  permanentFilter?: unknown
  exporter?: unknown
  basePath?: string
  selectedIds?: string[]
  onUnselectItems?: () => void
  showFilter?: boolean
  maxResults?: number
  total?: number
  ids?: string[]
}

export const SongListActions = ({
  className,
  resource,
  filters,
  displayedFilters,
  filterValues,
  showFilter,
  ...rest
}: SongListActionsProps) => {
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))
  return (
    <TopToolbar className={className} {...sanitizeListRestProps(rest)}>
      <ShuffleAllButton filters={filterValues as Record<string, unknown>} />
      {filters &&
        cloneElement(filters, {
          resource,
          showFilter,
          displayedFilters,
          filterValues,
          context: 'button',
        } as Record<string, unknown>)}
      {isNotSmall && <ToggleFieldsMenu resource="song" />}
    </TopToolbar>
  )
}
