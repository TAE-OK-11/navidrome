import React, { useCallback, useEffect } from 'react'
import { useSelector } from 'react-redux'
import {
  Layout as RALayout,
  useRefresh,
  useSetLocale,
  useSidebarState,
} from 'react-admin'
import makeStyles from '../themes/makeStyles'
import { HotKeys } from 'react-hotkeys'
import Menu from './Menu'
import AppBar from './AppBar'
import Notification from './Notification'
import { useSearchRefocus } from '../common'
import { retrieveTranslation } from '../i18n'
import config from '../config'

const useStyles = makeStyles({
  root: { paddingBottom: (props) => (props.addPadding ? '80px' : 0) },
})

const Layout = (props) => {
  const queue = useSelector((state) => state.player?.queue || [])
  const classes = useStyles({ addPadding: queue.length > 0 })
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

  return (
    <HotKeys handlers={{ TOGGLE_MENU: toggleMenu }}>
      <RALayout
        {...props}
        className={classes.root}
        menu={Menu}
        appBar={AppBar}
        notification={Notification}
      />
    </HotKeys>
  )
}

export default Layout
