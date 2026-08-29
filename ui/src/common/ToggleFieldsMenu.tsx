import React, { useState } from 'react'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import { Box, Typography } from '@mui/material'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import Checkbox from '@mui/material/Checkbox'
import { useDispatch, useSelector } from 'react-redux'
import { useTranslate } from 'react-admin'
import { setToggleableFields } from '../actions'

type ToggleFieldsMenuProps = {
  resource: string
  topbarComponent?: React.ElementType
  hideColumns?: boolean
}

export const ToggleFieldsMenu = ({
  resource,
  topbarComponent: TopBarComponent,
  hideColumns,
}: ToggleFieldsMenuProps) => {
  const [anchorEl, setAnchorEl] = useState(null)
  const dispatch = useDispatch()
  const translate = useTranslate()
  const toggleableColumns = useSelector(
    (state) => state.settings.toggleableFields[resource],
  )
  const omittedColumns =
    useSelector((state) => state.settings.omittedFields[resource]) || []

  const open = Boolean(anchorEl)

  const handleOpen = (event) => {
    setAnchorEl(event.currentTarget)
  }
  const handleClose = () => {
    setAnchorEl(null)
  }

  const handleClick = (selectedColumn) => {
    dispatch(
      setToggleableFields({
        [resource]: {
          ...toggleableColumns,
          [selectedColumn]: !toggleableColumns[selectedColumn],
        },
      }),
    )
  }

  return (
    <Box sx={{ position: 'relative', top: '-0.5em' }}>
      <IconButton
        aria-label="more"
        aria-controls="long-menu"
        aria-haspopup="true"
        onClick={handleOpen}
        size="large"
      >
        <MoreVertIcon />
      </IconButton>
      <Menu
        id="long-menu"
        anchorEl={anchorEl}
        keepMounted
        open={open}
        onClose={handleClose}
        slotProps={{
          paper: { sx: { width: '24ch' } },
        }}
      >
        {TopBarComponent && <TopBarComponent />}
        {!hideColumns && toggleableColumns ? (
          <Box>
            <Typography sx={{ m: '1rem' }}>
              {translate('ra.toggleFieldsMenu.columnsToDisplay')}
            </Typography>
            <Box sx={{ maxHeight: '21rem', overflow: 'auto' }}>
              {Object.entries(toggleableColumns).map(([key, val]) =>
                !omittedColumns.includes(key) ? (
                  <MenuItem key={key} onClick={() => handleClick(key)}>
                    <Checkbox checked={Boolean(val)} />
                    {translate(`resources.${resource}.fields.${key}`)}
                  </MenuItem>
                ) : null,
              )}
            </Box>
          </Box>
        ) : null}
      </Menu>
    </Box>
  )
}
