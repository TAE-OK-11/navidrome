import React, { useCallback, useEffect } from 'react'
import { useSelector } from 'react-redux'
import {
  Layout as RALayout,
  useRefresh,
  useSetLocale,
  useSidebarState,
} from 'react-admin'
import { styled } from '@mui/material/styles'
import { useAppHotkey } from '../hooks/useAppHotkey'
import Menu from './Menu'
import AppBar from './AppBar'
import Notification from './Notification'
import ClientError from './ClientError'
import { useSearchRefocus } from '../common'
import { retrieveTranslation } from '../i18n'
import config from '../config'

const StyledLayout = styled(RALayout, {
  shouldForwardProp: (prop) => prop !== 'addPadding',
})(({ addPadding }) => ({ paddingBottom: addPadding ? 80 : 0 }))

const Layout = (props) => {
  const queue = useSelector((state) => state.player?.queue || [])
  const [sidebarOpen, setSidebarOpen] = useSidebarState()
  const setLocale = useSetLocale()
  const refresh = useRefresh()
  useSearchRefocus()

  useEffect(() => {
    if (config.defaultLanguage !== '' && !localStorage.getItem('locale')) {
      retrieveTranslation(config.defaultLanguage)
        .then(() => setLocale(config.defaultLanguage))
        .then(() => {
          localStorage.setItem('locale', config.defaultLanguage)
          refresh()
        })
        .catch((e) => {
          // eslint-disable-next-line no-console
          console.error(
            'Cannot load language "' + config.defaultLanguage + '": ' + e,
          )
        })
    }
  }, [setLocale, refresh])

  const toggleMenu = useCallback(
    () => setSidebarOpen(!sidebarOpen),
    [sidebarOpen, setSidebarOpen],
  )

  useAppHotkey('TOGGLE_MENU', toggleMenu)

  return (
    <StyledLayout
      {...props}
      addPadding={queue.length > 0}
      menu={Menu}
      appBar={AppBar}
      error={ClientError}
      notification={Notification}
    />
  )
}

export default Layout
