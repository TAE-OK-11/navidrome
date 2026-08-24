// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useState } from 'react'
import TextField from '@mui/material/TextField'
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
  InputAdornment,
  IconButton,
} from '@mui/material'
import AddIcon from '@mui/icons-material/Add'
import { useGetList, useTranslate } from 'react-admin'
import PropTypes from 'prop-types'
import { isWritable } from '../common'

const PlaylistSearchField = ({
  searchText,
  onSearchChange,
  onCreateNew,
  onKeyDown,
  canCreateNew,
}) => {
  const translate = useTranslate()

  return (
    <TextField
      autoFocus
      variant="outlined"
      sx={{ mb: 2, width: '100%', flexShrink: 0 }}
      label={translate('resources.playlist.fields.name')}
      value={searchText}
      onChange={(e) => onSearchChange(e.target.value)}
      onKeyDown={onKeyDown}
      placeholder={translate('resources.playlist.actions.searchOrCreate')}
      slotProps={{
        input: {
          endAdornment: canCreateNew && (
            <InputAdornment position="end">
              <IconButton
                onClick={onCreateNew}
                size="small"
                title={translate('resources.playlist.actions.addNewPlaylist', {
                  name: searchText,
                })}
              >
                <AddIcon />
              </IconButton>
            </InputAdornment>
          ),
        },
      }}
    />
  )
}

const EmptyPlaylistMessage = ({ searchText, canCreateNew }) => {
  const translate = useTranslate()

  return (
    <Box sx={{ p: 2, textAlign: 'center', color: 'text.secondary' }}>
      <Typography variant="body2">
        {searchText
          ? translate('resources.playlist.message.noPlaylistsFound')
          : translate('resources.playlist.message.noPlaylists')}
      </Typography>
      {canCreateNew && (
        <Typography variant="body2" color="primary">
          {translate('resources.playlist.actions.pressEnterToCreate')}
        </Typography>
      )}
    </Box>
  )
}

const PlaylistListItem = ({ playlist, isSelected, onToggle }) => {
  return (
    <ListItemButton sx={{ py: 0 }} onClick={() => onToggle(playlist)} dense>
      <ListItemIcon>
        <Checkbox
          icon={<CheckBoxOutlineBlankIcon fontSize="small" />}
          checkedIcon={<CheckBoxIcon fontSize="small" />}
          checked={isSelected}
          tabIndex={-1}
          disableRipple
        />
      </ListItemIcon>
      <ListItemText primary={playlist.name} />
    </ListItemButton>
  )
}

const CreatePlaylistItem = ({ searchText, onCreateNew }) => {
  const translate = useTranslate()

  return (
    <ListItemButton sx={{ py: 0 }} onClick={onCreateNew} dense>
      <ListItemIcon>
        <AddIcon sx={{ fontSize: '1.25rem', m: '9px' }} />
      </ListItemIcon>
      <ListItemText
        primary={translate('resources.playlist.actions.addNewPlaylist', {
          name: searchText,
        })}
      />
    </ListItemButton>
  )
}

const PlaylistList = ({
  filteredOptions,
  selectedPlaylists,
  onPlaylistToggle,
  searchText,
  canCreateNew,
  onCreateNew,
}) => {
  const isPlaylistSelected = (playlist) =>
    selectedPlaylists.some((p) => p.id === playlist.id)

  return (
    <List
      sx={{
        flex: 1,
        minHeight: 0,
        overflow: 'auto',
        border: 1,
        borderColor: 'divider',
        borderRadius: 1,
        bgcolor: 'background.paper',
      }}
    >
      {filteredOptions.length === 0 ? (
        <EmptyPlaylistMessage
          searchText={searchText}
          canCreateNew={canCreateNew}
        />
      ) : (
        filteredOptions.map((playlist) => (
          <PlaylistListItem
            key={playlist.id}
            playlist={playlist}
            isSelected={isPlaylistSelected(playlist)}
            onToggle={onPlaylistToggle}
          />
        ))
      )}
      {canCreateNew && filteredOptions.length > 0 && (
        <CreatePlaylistItem searchText={searchText} onCreateNew={onCreateNew} />
      )}
    </List>
  )
}

const SelectedPlaylistChip = ({ playlist, onRemove }) => {
  const translate = useTranslate()

  return (
    <Box
      component="span"
      sx={{
        display: 'inline-flex',
        alignItems: 'center',
        m: 0.5,
        py: 0.5,
        px: 1,
        bgcolor: 'primary.main',
        color: 'primary.contrastText',
        borderRadius: 1,
        fontSize: '0.875rem',
      }}
    >
      {playlist.name}
      <IconButton
        sx={{ ml: 0.5, p: '2px', color: 'inherit' }}
        size="small"
        onClick={() => onRemove(playlist)}
        title={translate('resources.playlist.actions.removeFromSelection')}
      >
        {'×'}
      </IconButton>
    </Box>
  )
}

const SelectedPlaylistsDisplay = ({ selectedPlaylists, onRemoveSelected }) => {
  if (selectedPlaylists.length === 0) {
    return null
  }

  return (
    <Box sx={{ mt: 2, flexShrink: 0, maxHeight: '30%', overflow: 'auto' }}>
      <Box>
        {selectedPlaylists.map((playlist, index) => (
          <SelectedPlaylistChip
            key={playlist.id || `new-${index}`}
            playlist={playlist}
            onRemove={onRemoveSelected}
          />
        ))}
      </Box>
    </Box>
  )
}

export const SelectPlaylistInput = ({ onChange }) => {
  const [searchText, setSearchText] = useState('')
  const [selectedPlaylists, setSelectedPlaylists] = useState([])

  const { data = [] } = useGetList('playlist', {
    pagination: { page: 1, perPage: -1 },
    sort: { field: 'name', order: 'ASC' },
    filter: { smart: false },
  })

  const options = data.filter((option) => isWritable(option.ownerId))

  // Filter playlists based on search text
  const filteredOptions =
    options?.filter((option) =>
      option.name.toLowerCase().includes(searchText.toLowerCase()),
    ) || []

  const handlePlaylistToggle = (playlist) => {
    const isSelected = selectedPlaylists.some((p) => p.id === playlist.id)
    let newSelection

    if (isSelected) {
      newSelection = selectedPlaylists.filter((p) => p.id !== playlist.id)
    } else {
      newSelection = [...selectedPlaylists, playlist]
    }

    setSelectedPlaylists(newSelection)
    onChange(newSelection)
  }

  const handleRemoveSelected = (playlistToRemove) => {
    const newSelection = selectedPlaylists.filter(
      (p) => p.id !== playlistToRemove.id,
    )
    setSelectedPlaylists(newSelection)
    onChange(newSelection)
  }

  const handleCreateNew = () => {
    if (searchText.trim()) {
      const newPlaylist = { name: searchText.trim() }
      const newSelection = [...selectedPlaylists, newPlaylist]
      setSelectedPlaylists(newSelection)
      onChange(newSelection)
      setSearchText('')
    }
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && searchText.trim()) {
      e.preventDefault()
      handleCreateNew()
    }
  }

  const canCreateNew = Boolean(
    searchText.trim() &&
    !filteredOptions.some(
      (option) => option.name.toLowerCase() === searchText.toLowerCase().trim(),
    ) &&
    !selectedPlaylists.some((p) => p.name === searchText.trim()),
  )

  return (
    <Box
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <PlaylistSearchField
        searchText={searchText}
        onSearchChange={setSearchText}
        onCreateNew={handleCreateNew}
        onKeyDown={handleKeyDown}
        canCreateNew={canCreateNew}
      />

      <PlaylistList
        filteredOptions={filteredOptions}
        selectedPlaylists={selectedPlaylists}
        onPlaylistToggle={handlePlaylistToggle}
        searchText={searchText}
        canCreateNew={canCreateNew}
        onCreateNew={handleCreateNew}
      />

      <SelectedPlaylistsDisplay
        selectedPlaylists={selectedPlaylists}
        onRemoveSelected={handleRemoveSelected}
      />
    </Box>
  )
}

SelectPlaylistInput.propTypes = {
  onChange: PropTypes.func.isRequired,
}

// PropTypes for sub-components
PlaylistSearchField.propTypes = {
  searchText: PropTypes.string.isRequired,
  onSearchChange: PropTypes.func.isRequired,
  onCreateNew: PropTypes.func.isRequired,
  onKeyDown: PropTypes.func.isRequired,
  canCreateNew: PropTypes.bool.isRequired,
}

EmptyPlaylistMessage.propTypes = {
  searchText: PropTypes.string.isRequired,
  canCreateNew: PropTypes.bool.isRequired,
}

PlaylistListItem.propTypes = {
  playlist: PropTypes.object.isRequired,
  isSelected: PropTypes.bool.isRequired,
  onToggle: PropTypes.func.isRequired,
}

CreatePlaylistItem.propTypes = {
  searchText: PropTypes.string.isRequired,
  onCreateNew: PropTypes.func.isRequired,
}

PlaylistList.propTypes = {
  filteredOptions: PropTypes.array.isRequired,
  selectedPlaylists: PropTypes.array.isRequired,
  onPlaylistToggle: PropTypes.func.isRequired,
  searchText: PropTypes.string.isRequired,
  canCreateNew: PropTypes.bool.isRequired,
  onCreateNew: PropTypes.func.isRequired,
}

SelectedPlaylistChip.propTypes = {
  playlist: PropTypes.object.isRequired,
  onRemove: PropTypes.func.isRequired,
}

SelectedPlaylistsDisplay.propTypes = {
  selectedPlaylists: PropTypes.array.isRequired,
  onRemoveSelected: PropTypes.func.isRequired,
}
