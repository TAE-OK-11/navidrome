// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useState, useEffect, useCallback } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useDataProvider, useTranslate, useRefresh } from 'react-admin'
import {
  Box,
  Chip,
  ClickAwayListener,
  FormControl,
  FormGroup,
  FormControlLabel,
  Checkbox,
  Typography,
  Paper,
  Popper,
} from '@mui/material'
import { ExpandMore, ExpandLess, LibraryMusic } from '@mui/icons-material'
import { setSelectedLibraries, setUserLibraries } from '../actions'
import { useRefreshOnEvents } from './useRefreshOnEvents'

const LibrarySelector = () => {
  const dispatch = useDispatch()
  const dataProvider = useDataProvider()
  const translate = useTranslate()
  const refresh = useRefresh()
  const [anchorEl, setAnchorEl] = useState(null)
  const [open, setOpen] = useState(false)

  const { userLibraries, selectedLibraries } = useSelector(
    (state) => state.library,
  )

  // Load user's libraries when component mounts
  const loadUserLibraries = useCallback(async () => {
    const userId = localStorage.getItem('userId')
    if (userId) {
      try {
        const { data } = await dataProvider.getOne('user', { id: userId })
        const libraries = data.libraries || []
        dispatch(setUserLibraries(libraries))
      } catch (error) {
        // eslint-disable-next-line no-console
        console.warn(
          'Could not load user libraries (this may be expected for non-admin users):',
          error,
        )
      }
    }
  }, [dataProvider, dispatch])

  // Initial load
  useEffect(() => {
    loadUserLibraries()
  }, [loadUserLibraries])

  // Reload user libraries when library changes occur
  useRefreshOnEvents({
    events: ['library', 'user'],
    onRefresh: loadUserLibraries,
  })

  // Don't render if user has no libraries or only has one library
  if (!userLibraries.length || userLibraries.length === 1) {
    return null
  }

  const handleToggle = (event) => {
    setAnchorEl(event.currentTarget)
    const wasOpen = open
    setOpen(!open)
    // Refresh data when closing the dropdown
    if (wasOpen) {
      refresh()
    }
  }

  const handleClose = () => {
    setOpen(false)
    refresh()
  }

  const handleLibraryToggle = (libraryId) => {
    const newSelection = selectedLibraries.includes(libraryId)
      ? selectedLibraries.filter((id) => id !== libraryId)
      : [...selectedLibraries, libraryId]

    dispatch(setSelectedLibraries(newSelection))
  }

  const handleMasterCheckboxChange = () => {
    if (isAllSelected) {
      dispatch(setSelectedLibraries([]))
    } else {
      const allIds = userLibraries.map((lib) => lib.id)
      dispatch(setSelectedLibraries(allIds))
    }
  }

  const selectedCount = selectedLibraries.length
  const totalCount = userLibraries.length
  const isAllSelected = selectedCount === totalCount
  const isNoneSelected = selectedCount === 0
  const isIndeterminate = selectedCount > 0 && selectedCount < totalCount

  const displayText = isNoneSelected
    ? translate('menu.librarySelector.none') + ` (0 of ${totalCount})`
    : isAllSelected
      ? translate('menu.librarySelector.allLibraries', { count: totalCount })
      : translate('menu.librarySelector.multipleLibraries', {
          selected: selectedCount,
          total: totalCount,
        })

  return (
    <Box sx={{ my: 3, px: 2, display: 'flex', justifyContent: 'center' }}>
      <Chip
        icon={<LibraryMusic />}
        label={displayText}
        onClick={handleToggle}
        onDelete={open ? handleToggle : undefined}
        deleteIcon={open ? <ExpandLess /> : <ExpandMore />}
        variant="outlined"
        sx={{
          borderRadius: 1,
          height: 38.4,
          fontSize: '1rem',
          fontWeight: 'normal',
          minWidth: 210,
          justifyContent: 'flex-start',
          px: 1,
          mt: 0.1,
          '& .MuiChip-label': { pl: 2, pr: 1 },
          '& .MuiChip-icon': { fontSize: '1.2rem', ml: 0.5 },
        }}
      />

      <Popper
        open={open}
        anchorEl={anchorEl}
        placement="bottom-start"
        sx={{ zIndex: 1300 }}
      >
        <ClickAwayListener onClickAway={handleClose}>
          <Paper sx={{ p: 2, mt: 1, minWidth: 300, maxWidth: 400 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 0 }}>
              <Checkbox
                checked={isAllSelected}
                indeterminate={isIndeterminate}
                onChange={handleMasterCheckboxChange}
                size="small"
                sx={{ p: '7px', ml: '-9px', mr: 0 }}
              />
              <Typography>
                {translate('menu.librarySelector.selectLibraries')}:
              </Typography>
            </Box>

            <FormControl component="fieldset" variant="standard" fullWidth>
              <FormGroup>
                {userLibraries.map((library) => (
                  <FormControlLabel
                    key={library.id}
                    control={
                      <Checkbox
                        checked={selectedLibraries.includes(library.id)}
                        onChange={() => handleLibraryToggle(library.id)}
                        size="small"
                      />
                    }
                    label={library.name}
                  />
                ))}
              </FormGroup>
            </FormControl>
          </Paper>
        </ClickAwayListener>
      </Popper>
    </Box>
  )
}

export default LibrarySelector
