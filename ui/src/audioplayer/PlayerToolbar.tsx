// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useCallback } from 'react'
import { useDispatch } from 'react-redux'
import { useGetOne } from 'react-admin'
import { GlobalHotKeys } from 'react-hotkeys'
import IconButton from '@mui/material/IconButton'
import { Box, useMediaQuery } from '@mui/material'
import { RiSaveLine } from 'react-icons/ri'
import { LoveButton, useToggleLove } from '../common'
import { openSaveQueueDialog } from '../actions'
import { keyMap } from '../hotkeys'

const desktopItemSx = {
  display: 'flex',
  alignItems: 'center',
  flexGrow: 1,
  justifyContent: 'flex-end',
  gap: '0.5rem',
  listStyle: 'none',
  p: 0,
  m: 0,
}

const mobileItemSx = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  listStyle: 'none',
  p: 0.5,
  m: 0,
  height: 24,
}

const desktopButtonSx = {
  width: '2.5rem',
  height: '2.5rem',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  p: 0,
}

const mobileButtonSx = {
  width: 24,
  height: 24,
  p: 0,
  m: 0,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  fontSize: 18,
}

const PlayerToolbar = ({ id, isRadio }) => {
  const dispatch = useDispatch()
  const { data, isPending } = useGetOne(
    'song',
    { id },
    { enabled: !!id && !isRadio },
  )
  const [toggleLove, toggling] = useToggleLove('song', data)
  const isDesktop = useMediaQuery('(min-width:810px)')

  const handlers = {
    TOGGLE_LOVE: useCallback(() => toggleLove(), [toggleLove]),
  }

  const handleSaveQueue = useCallback(
    (e) => {
      dispatch(openSaveQueueDialog())
      e.stopPropagation()
    },
    [dispatch],
  )

  const buttonSx = isDesktop ? desktopButtonSx : mobileButtonSx

  const saveQueueButton = (
    <IconButton
      size={isDesktop ? 'small' : undefined}
      onClick={handleSaveQueue}
      disabled={isRadio}
      data-testid="save-queue-button"
      sx={buttonSx}
    >
      <RiSaveLine
        style={!isDesktop ? { fontSize: 18, display: 'flex' } : undefined}
      />
    </IconButton>
  )

  const loveButton = (
    <LoveButton
      record={data}
      resource={'song'}
      size={isDesktop ? undefined : 'inherit'}
      disabled={isPending || toggling || !id || isRadio}
      sx={buttonSx}
    />
  )

  return (
    <>
      <GlobalHotKeys keyMap={keyMap} handlers={handlers} allowChanges />
      {isDesktop ? (
        <Box component="li" className="toolbar item" sx={desktopItemSx}>
          {saveQueueButton}
          {loveButton}
        </Box>
      ) : (
        <>
          <Box component="li" className="mobileListItem item" sx={mobileItemSx}>
            {saveQueueButton}
          </Box>
          <Box component="li" className="mobileListItem item" sx={mobileItemSx}>
            {loveButton}
          </Box>
        </>
      )}
    </>
  )
}

export default PlayerToolbar
