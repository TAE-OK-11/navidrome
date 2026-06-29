import { useSelector } from 'react-redux'
import useMediaQuery from '@material-ui/core/useMediaQuery'
import themes, { findThemeKeyByDisplayName, getTheme } from './index'
import { AUTO_THEME_ID } from '../consts'
import config from '../config'
import { useEffect } from 'react'

const fallbackThemeKey = findThemeKeyByDisplayName(config.defaultTheme)

const useCurrentTheme = () => {
  const prefersLightMode = useMediaQuery('(prefers-color-scheme: light)')
  const theme = useSelector((state) => {
    if (state.theme === AUTO_THEME_ID) {
      return prefersLightMode ? themes.LightTheme : themes.DarkTheme
    }
    return (
      getTheme(state.theme) || getTheme(fallbackThemeKey) || themes.DarkTheme
    )
  })

  useEffect(() => {
    let style = document.getElementById('nd-player-style-override')
    if (theme.player.stylesheet) {
      if (style === null) {
        style = document.createElement('style')
        style.id = 'nd-player-style-override'
        style.innerHTML = theme.player.stylesheet
        document.head.appendChild(style)
      } else {
        style.innerHTML = theme.player.stylesheet
      }
    } else {
      if (style !== null) {
        document.head.removeChild(style)
      }
    }

    // Set body background color to match theme (fixes white background on pull-to-refresh)
    const isDark = theme.palette?.type === 'dark'
    const bgColor =
      theme.palette?.background?.default || (isDark ? '#303030' : '#fafafa')
    document.body.style.backgroundColor = bgColor
  }, [theme])

  return theme
}

export default useCurrentTheme
