// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useState, useEffect, useMemo } from 'react'
import Checkbox from '@mui/material/Checkbox'
import CheckBoxIcon from '@mui/icons-material/CheckBox'
import CheckBoxOutlineBlankIcon from '@mui/icons-material/CheckBoxOutlineBlank'
import {
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Typography,
  Box,
} from '@mui/material'
import { useGetList, useTranslate } from 'react-admin'
import PropTypes from 'prop-types'

const EmptyLibraryMessage = () => {
  return (
    <Box sx={{ p: 2, textAlign: 'center', color: 'text.secondary' }}>
      <Typography variant="body2">No libraries available</Typography>
    </Box>
  )
}

const LibraryListItem = ({ library, isSelected, onToggle }) => {
  return (
    <ListItemButton sx={{ py: 0 }} onClick={() => onToggle(library)} dense>
      <ListItemIcon>
        <Checkbox
          icon={<CheckBoxOutlineBlankIcon fontSize="small" />}
          checkedIcon={<CheckBoxIcon fontSize="small" />}
          checked={isSelected}
          tabIndex={-1}
          disableRipple
        />
      </ListItemIcon>
      <ListItemText primary={library.name} />
    </ListItemButton>
  )
}

export const SelectLibraryInput = ({
  onChange,
  value = [],
  isNewUser = false,
}) => {
  const translate = useTranslate()
  const [selectedLibraryIds, setSelectedLibraryIds] = useState([])
  const [hasInitialized, setHasInitialized] = useState(false)

  const {
    data = [],
    ids,
    isPending,
    isLoading,
  } = useGetList('library', {
    pagination: { page: 1, perPage: -1 },
    sort: { field: 'name', order: 'ASC' },
    filter: {},
  })
  const loading = isPending ?? isLoading ?? false

  const options = useMemo(
    () =>
      Array.isArray(data) ? data : (ids && ids.map((id) => data[id])) || [],
    [ids, data],
  )

  // Reset initialization state when isNewUser changes
  useEffect(() => {
    if (isNewUser) {
      setHasInitialized(false)
    }
  }, [isNewUser])

  // Pre-select default libraries for new users
  useEffect(() => {
    if (
      isNewUser &&
      !loading &&
      options.length > 0 &&
      !hasInitialized &&
      Array.isArray(value) &&
      value.length === 0
    ) {
      const defaultLibraryIds = options
        .filter((lib) => lib.defaultNewUsers)
        .map((lib) => lib.id)

      if (defaultLibraryIds.length > 0) {
        setSelectedLibraryIds(defaultLibraryIds)
        onChange(defaultLibraryIds)
      }

      setHasInitialized(true)
    }
  }, [isNewUser, loading, options, hasInitialized, value, onChange])

  // Update selectedLibraryIds when value prop changes (for editing mode and pre-selection)
  useEffect(() => {
    // For new users, only sync from value prop if it has actual data
    // This prevents empty initial state from overriding our pre-selection
    if (isNewUser && Array.isArray(value) && value.length === 0) {
      return
    }

    if (Array.isArray(value)) {
      const libraryIds = value.map((item) =>
        typeof item === 'object' ? item.id : item,
      )
      setSelectedLibraryIds(libraryIds)
    } else if (value.length === 0) {
      // Handle case where value is explicitly set to empty array (for existing users)
      setSelectedLibraryIds([])
    }
  }, [value, isNewUser, hasInitialized])

  const isLibrarySelected = (library) => selectedLibraryIds.includes(library.id)

  const handleLibraryToggle = (library) => {
    const isSelected = selectedLibraryIds.includes(library.id)
    let newSelection

    if (isSelected) {
      newSelection = selectedLibraryIds.filter((id) => id !== library.id)
    } else {
      newSelection = [...selectedLibraryIds, library.id]
    }

    setSelectedLibraryIds(newSelection)
    onChange(newSelection)
  }

  const handleMasterCheckboxChange = () => {
    const isAllSelected = selectedLibraryIds.length === options.length
    const newSelection = isAllSelected ? [] : options.map((lib) => lib.id)

    setSelectedLibraryIds(newSelection)
    onChange(newSelection)
  }

  const selectedCount = selectedLibraryIds.length
  const totalCount = options.length
  const isAllSelected = selectedCount === totalCount && totalCount > 0
  const isIndeterminate = selectedCount > 0 && selectedCount < totalCount

  return (
    <Box sx={{ width: 960, maxWidth: '100%' }}>
      {options.length > 1 && (
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1, pl: 1 }}>
          <Checkbox
            checked={isAllSelected}
            indeterminate={isIndeterminate}
            onChange={handleMasterCheckboxChange}
            size="small"
            sx={{ p: '7px', ml: '-9px', mr: 1 }}
          />
          <Typography variant="body2">
            {translate('resources.user.message.selectAllLibraries')}
          </Typography>
        </Box>
      )}
      <List
        sx={{
          height: 120,
          overflow: 'auto',
          border: 1,
          borderColor: 'divider',
          borderRadius: 1,
          bgcolor: 'background.paper',
        }}
      >
        {options.length === 0 ? (
          <EmptyLibraryMessage />
        ) : (
          options.map((library) => (
            <LibraryListItem
              key={library.id}
              library={library}
              isSelected={isLibrarySelected(library)}
              onToggle={handleLibraryToggle}
            />
          ))
        )}
      </List>
    </Box>
  )
}

SelectLibraryInput.propTypes = {
  onChange: PropTypes.func.isRequired,
  value: PropTypes.array,
  isNewUser: PropTypes.bool,
}

LibraryListItem.propTypes = {
  library: PropTypes.object.isRequired,
  isSelected: PropTypes.bool.isRequired,
  onToggle: PropTypes.func.isRequired,
}
