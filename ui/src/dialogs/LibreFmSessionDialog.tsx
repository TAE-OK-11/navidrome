import React, {
  createRef,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  LinearProgress,
  Link,
  TextField,
} from '@mui/material'
import { useNotify, useTranslate } from 'react-admin'
import { useDispatch, useSelector } from 'react-redux'
import { closeLibreFmSessionDialog } from '../actions'
import { httpClient } from '../dataProvider'
import { baseUrl, openInNewTab } from '../utils'
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
  const [apiKey, setApiKey] = useState('')
  const [authUrl, setAuthUrl] = useState('https://libre.fm/api/auth/')
  const [authPolling, setAuthPolling] = useState(false)
  const openedTab = useRef<Window | null>(null)
  const inputRef = createRef<HTMLInputElement>()

  useEffect(() => {
    if (!authPolling) {
      return
    }
    let checks = 30
    const interval = window.setInterval(() => {
      httpClient('/api/librefm/link')
        .then((response) => {
          if (response.json.status === true) {
            setLinked(true)
            setAuthPolling(false)
            notify('message.libreFmLinkSuccess', { type: 'success' })
            openedTab.current?.close()
            dispatch(closeLibreFmSessionDialog())
          } else if (openedTab.current?.closed === true || --checks <= 0) {
            setAuthPolling(false)
          }
        })
        .catch(() => {
          setAuthPolling(false)
        })
    }, 2000)
    return () => window.clearInterval(interval)
  }, [authPolling, dispatch, notify, setLinked])

  const handleChange = (event) => {
    setSessionKey(event.target.value)
  }

  const handleLinkClick = (event: React.MouseEvent) => {
    inputRef.current?.focus()
  }

  const closeDialog = useCallback(() => {
    openedTab.current?.close()
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

  const startBrowserAuth = () => {
    let tab
    try {
      tab = openInNewTab('about:blank')
    } catch {
      notify('message.libreFmLinkFailure', { type: 'warning' })
      return
    }
    openedTab.current = tab
    httpClient('/api/librefm/link')
      .then((response) => {
        const linkToken = response.json.linkToken
        const key = response.json.apiKey || apiKey
        const auth = response.json.authUrl || authUrl
        if (!linkToken || !key) {
          tab?.close()
          notify('message.libreFmLinkFailure', { type: 'warning' })
          return
        }
        setApiKey(key)
        setAuthUrl(auth)
        const callbackEndpoint = baseUrl(
          `/api/librefm/link/callback?uid=${encodeURIComponent(linkToken)}`,
        )
        const callbackUrl = `${window.location.origin}${callbackEndpoint}`
        tab.location.href = `${auth}?api_key=${encodeURIComponent(key)}&cb=${encodeURIComponent(callbackUrl)}`
        setAuthPolling(true)
      })
      .catch(() => {
        tab?.close()
        notify('message.libreFmLinkFailure', { type: 'warning' })
      })
  }

  const handleClickClose = (event) => {
    if (!checking && !authPolling) {
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
          {translate('resources.user.message.libreFmSessionKey')}{' '}
          <Link
            href="https://libre.fm/settings.php"
            onClick={handleLinkClick}
            target="_blank"
            rel="noopener noreferrer"
          >
            {translate('resources.user.message.clickHereForSessionKey')}
          </Link>
        </DialogContentText>
        <TextField
          value={sessionKey}
          onKeyPress={handleKeyPress}
          onChange={handleChange}
          disabled={checking || authPolling}
          required
          autoFocus
          fullWidth={true}
          variant={'outlined'}
          label={translate('resources.user.fields.sessionKey')}
          inputRef={inputRef}
          sx={{ mt: 2 }}
        />
        {checking && <LinearProgress />}
        {authPolling && <LinearProgress />}
      </DialogContent>
      <DialogActions>
        <Button
          onClick={handleClickClose}
          disabled={checking || authPolling}
          color="primary"
        >
          {translate('ra.action.cancel')}
        </Button>
        <Button
          onClick={startBrowserAuth}
          disabled={checking || authPolling}
          color="primary"
        >
          {translate('resources.user.message.libreFmBrowserAuth')}
        </Button>
        <Button
          onClick={handleSave}
          disabled={checking || authPolling || sessionKey === ''}
          color="primary"
          data-testid="librefm-session-save"
        >
          {translate('ra.action.save')}
        </Button>
      </DialogActions>
    </Dialog>
  )
}
