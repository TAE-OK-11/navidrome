// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { useSelector } from 'react-redux'
import useMediaQuery from '@mui/material/useMediaQuery'
import themes, { findThemeKeyByDisplayName, getTheme } from './index'
import { AUTO_THEME_ID } from '../consts'
import config from '../config'
import { useEffect, useMemo } from 'react'

const fallbackThemeKey = findThemeKeyByDisplayName(config.defaultTheme)

const useCurrentTheme = () => {
  // Runs above the ThemeProvider carrying the prop below, so it needs its own noSsr or the
  // auto theme renders dark first and flips.
  const prefersLightMode = useMediaQuery('(prefers-color-scheme: light)', {
    noSsr: true,
  })
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

  // We never server-render, so let media queries resolve on the first render: the default
  // defers them to an effect, which makes every mount paint the wrong breakpoint and reflow.
  return useMemo(
    () => ({
      ...theme,
      props: { ...theme.props, MuiUseMediaQuery: { noSsr: true } },
    }),
    [theme],
  )
}

export default useCurrentTheme
