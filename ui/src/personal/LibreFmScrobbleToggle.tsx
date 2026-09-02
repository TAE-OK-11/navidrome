import { useEffect, useRef, useState } from 'react'
import { useNotify, useTranslate } from 'react-admin'
import {
  FormControl,
  FormControlLabel,
  FormHelperText,
  LinearProgress,
  Link,
  Switch,
} from '@mui/material'
import { useDispatch } from 'react-redux'
import { useInterval } from '../common'
import { baseUrl, openInNewTab } from '../utils'
import { httpClient } from '../dataProvider'
import { LibreFmSessionDialog } from '../dialogs'
import { openLibreFmSessionDialog } from '../actions'

const Progress = (props) => {
  const { setLinked, setCheckingLink, openedTab } = props
  const notify = useNotify()
  let linkCheckDelay: number | null = 2000
  let linkChecks = 30

  const endChecking = (success) => {
    linkCheckDelay = null
    setCheckingLink(false)
    if (success) {
      notify('message.libreFmLinkSuccess', { type: 'success' })
    } else {
      notify('message.libreFmLinkFailure', { type: 'warning' })
    }
    setLinked(success)
  }

  useInterval(() => {
    httpClient('/api/librefm/link')
      .then((response) => {
        let result = false
        if (response.json.status === true) {
          result = true
          endChecking(true)
        }
        return result
      })
      .then((result) => {
        if (!result && openedTab.current?.closed === true) {
          endChecking(false)
          result = true
        }
        return result
      })
      .then((result) => {
        if (!result && --linkChecks === 0) {
          endChecking(false)
        }
      })
      .catch(() => {
        endChecking(false)
      })
  }, linkCheckDelay)

  return <LinearProgress />
}

export const LibreFmScrobbleToggle = () => {
  const dispatch = useDispatch()
  const notify = useNotify()
  const translate = useTranslate()
  const [linked, setLinked] = useState<boolean | null>(null)
  const [checkingLink, setCheckingLink] = useState(false)
  const [apiKey, setApiKey] = useState('')
  const [authUrl, setAuthUrl] = useState('https://libre.fm/api/auth/')
  const openedTab = useRef<Window | null>(null)

  useEffect(() => {
    httpClient('/api/librefm/link')
      .then((response) => {
        setLinked(response.json.status === true)
        setApiKey(response.json.apiKey || '')
        setAuthUrl(response.json.authUrl || 'https://libre.fm/api/auth/')
      })
      .catch(() => {
        setLinked(false)
      })
  }, [])

  const startLink = () => {
    let tab
    try {
      tab = openInNewTab('about:blank')
    } catch {
      notify('message.libreFmLinkFailure', { type: 'warning' })
      return
    }
    openedTab.current = tab
    setCheckingLink(true)
    httpClient('/api/librefm/link')
      .then((response) => {
        const linkToken = response.json.linkToken
        const key = response.json.apiKey || apiKey
        const auth = response.json.authUrl || authUrl
        if (!linkToken || !key) {
          tab?.close()
          notify('message.libreFmLinkFailure', { type: 'warning' })
          setCheckingLink(false)
          return
        }
        setApiKey(key)
        setAuthUrl(auth)
        const callbackEndpoint = baseUrl(
          `/api/librefm/link/callback?uid=${encodeURIComponent(linkToken)}`,
        )
        const callbackUrl = `${window.location.origin}${callbackEndpoint}`
        tab.location.href = `${auth}?api_key=${encodeURIComponent(key)}&cb=${encodeURIComponent(callbackUrl)}`
      })
      .catch(() => {
        tab?.close()
        notify('message.libreFmLinkFailure', { type: 'warning' })
        setCheckingLink(false)
      })
  }

  const toggleScrobble = () => {
    if (!linked) {
      startLink()
    } else {
      httpClient('/api/librefm/link', { method: 'DELETE' })
        .then(() => {
          setLinked(false)
          notify('message.libreFmUnlinkSuccess', { type: 'success' })
        })
        .catch(() =>
          notify('message.libreFmUnlinkFailure', { type: 'warning' }),
        )
    }
  }

  return (
    <>
      <FormControl>
        <FormControlLabel
          control={
            <Switch
              id={'librefm'}
              color="primary"
              checked={linked || checkingLink}
              disabled={linked === null || checkingLink}
              onChange={toggleScrobble}
            />
          }
          label={
            <span>{translate('menu.personal.options.libreFmScrobbling')}</span>
          }
        />
        {checkingLink && (
          <Progress
            setLinked={setLinked}
            setCheckingLink={setCheckingLink}
            openedTab={openedTab}
          />
        )}
        {!linked && !checkingLink && (
          <FormHelperText>
            <Link
              component="button"
              type="button"
              onClick={() => dispatch(openLibreFmSessionDialog())}
            >
              {translate('menu.personal.options.libreFmManualSessionKey')}
            </Link>
          </FormHelperText>
        )}
      </FormControl>
      <LibreFmSessionDialog setLinked={setLinked} />
    </>
  )
}
