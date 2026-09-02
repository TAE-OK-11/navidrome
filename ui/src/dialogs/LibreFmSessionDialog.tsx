import React, { createRef, useCallback, useState } from 'react'
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  LinearProgress,
  TextField,
} from '@mui/material'
import { useNotify, useTranslate } from 'react-admin'
import { useDispatch, useSelector } from 'react-redux'
import { closeLibreFmSessionDialog } from '../actions'
import { httpClient } from '../dataProvider'
import type { NavidromeRootState } from '../types/redux'

export const LibreFmSessionDialog = ({ setLinked }) => {
  const dispatch = useDispatch()
  const notify = useNotify()
  const translate = useTranslate()
  const { open } = useSelector(
    (state: NavidromeRootState) => state.libreFmSessionDialog,
  )
  const [sessionKey, setSessionKey] = useState('')
  const [checking, setChecking] = useState(false)
  const inputRef = createRef<HTMLInputElement>()

  const handleChange = (event) => {
    setSessionKey(event.target.value)
  }

  const closeDialog = useCallback(() => {
    dispatch(closeLibreFmSessionDialog())
  }, [dispatch])

  const handleSave = useCallback(
    (event) => {
      setChecking(true)
      httpClient('/api/librefm/link', {
        method: 'PUT',
        body: JSON.stringify({ sessionKey }),
      })
        .then((response) => {
          notify('message.libreFmLinkSuccess', {
            type: 'success',
            messageArgs: { user: response.json.user },
          })
          setLinked(true)
          setSessionKey('')
        })
        .catch((error) => {
          notify('message.libreFmLinkFailure', {
            type: 'warning',
            messageArgs: { error: error.body?.error || error.message },
          })
          setLinked(false)
        })
        .finally(() => {
          setChecking(false)
          closeDialog()
          event.stopPropagation()
        })
    },
    [closeDialog, notify, sessionKey, setLinked],
  )

  const handleClickClose = (event) => {
    if (!checking) {
      closeDialog()
      event.stopPropagation()
    }
  }

  const handleKeyPress = useCallback(
    (event) => {
      if (event.key === 'Enter' && sessionKey !== '') {
        handleSave(event)
      }
    },
    [sessionKey, handleSave],
  )

  return (
    <Dialog
      open={open}
      onClose={handleClickClose}
      aria-labelledby="form-dialog-librefm-session"
      fullWidth={true}
      maxWidth="md"
    >
      <DialogTitle id="form-dialog-librefm-session">Libre.fm</DialogTitle>
      <DialogContent>
        <DialogContentText>
          {translate('resources.user.message.libreFmSessionKey')}
        </DialogContentText>
        <TextField
          value={sessionKey}
          onKeyPress={handleKeyPress}
          onChange={handleChange}
          disabled={checking}
          required
          autoFocus
          fullWidth={true}
          variant={'outlined'}
          label={translate('resources.user.fields.sessionKey')}
          inputRef={inputRef}
          sx={{ mt: 2 }}
        />
        {checking && <LinearProgress />}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClickClose} disabled={checking} color="primary">
          {translate('ra.action.cancel')}
        </Button>
        <Button
          onClick={handleSave}
          disabled={checking || sessionKey === ''}
          color="primary"
          data-testid="librefm-session-save"
        >
          {translate('ra.action.save')}
        </Button>
      </DialogActions>
    </Dialog>
  )
}
