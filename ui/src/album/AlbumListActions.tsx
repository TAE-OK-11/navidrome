// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
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

const AlbumViewToggler = React.forwardRef(
  ({ showTitle = true, disableElevation, fullWidth }, ref) => {
    const dispatch = useDispatch()
    const albumView = useSelector((state) => state.albumView)
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

const AlbumListActions = ({
  currentSort,
  className,
  resource,
  filters,
  displayedFilters,
  filterValues,
  permanentFilter,
  exporter,
  basePath,
  selectedIds = [],
  onUnselectItems = () => null,
  showFilter,
  maxResults,
  total,
  fullWidth,
  ...rest
}) => {
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))
  const albumView = useSelector((state) => state.albumView)
  return (
    <TopToolbar className={className} {...sanitizeListRestProps(rest)}>
      {filters &&
        cloneElement(filters, {
          resource,
          showFilter,
          displayedFilters,
          filterValues,
          context: 'button',
        })}
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
