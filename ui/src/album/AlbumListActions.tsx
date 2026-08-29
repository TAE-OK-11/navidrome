import React, { cloneElement } from 'react'
import {
  Button,
  sanitizeListRestProps,
  TopToolbar,
  useTranslate,
} from 'react-admin'
import { Box, ButtonGroup, useMediaQuery, Typography } from '@mui/material'
import ViewHeadlineIcon from '@mui/icons-material/ViewHeadline'
import ViewModuleIcon from '@mui/icons-material/ViewModule'
import { useDispatch, useSelector } from 'react-redux'
import { albumViewGrid, albumViewTable } from '../actions'
import { ToggleFieldsMenu } from '../common'
import type { AppState } from '../types/redux'

type AlbumViewTogglerProps = {
  showTitle?: boolean
  disableElevation?: boolean
  fullWidth?: boolean
}

const AlbumViewToggler = React.forwardRef<HTMLDivElement, AlbumViewTogglerProps>(
  ({ showTitle = true }, ref) => {
    const dispatch = useDispatch()
    const albumView = useSelector((state: AppState) => state.albumView)
    const translate = useTranslate()
    return (
      <Box ref={ref}>
        {showTitle && (
          <Typography sx={{ m: '1rem' }}>
            {translate('ra.toggleFieldsMenu.layout')}
          </Typography>
        )}
        <ButtonGroup
          variant="text"
          color="primary"
          aria-label="text primary button group"
          sx={{ width: '100%', justifyContent: 'center' }}
        >
          <Button
            size="small"
            sx={{ pr: '0.5rem' }}
            label={translate('ra.toggleFieldsMenu.grid')}
            color={albumView.grid ? 'primary' : 'secondary'}
            onClick={() => dispatch(albumViewGrid())}
          >
            <ViewModuleIcon fontSize="inherit" />
          </Button>
          <Button
            size="small"
            sx={{ pl: '0.5rem' }}
            label={translate('ra.toggleFieldsMenu.table')}
            color={albumView.grid ? 'secondary' : 'primary'}
            onClick={() => dispatch(albumViewTable())}
          >
            <ViewHeadlineIcon fontSize="inherit" />
          </Button>
        </ButtonGroup>
      </Box>
    )
  },
)

AlbumViewToggler.displayName = 'AlbumViewToggler'

type AlbumListActionsProps = {
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
  fullWidth?: boolean
}

const AlbumListActions = ({
  className,
  resource,
  filters,
  displayedFilters,
  filterValues,
  showFilter,
  ...rest
}: AlbumListActionsProps) => {
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))
  const albumView = useSelector((state: AppState) => state.albumView)
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
      {isNotSmall ? (
        <ToggleFieldsMenu
          resource="album"
          topbarComponent={AlbumViewToggler}
          hideColumns={albumView.grid}
        />
      ) : (
        <AlbumViewToggler showTitle={false} />
      )}
    </TopToolbar>
  )
}

export default AlbumListActions
