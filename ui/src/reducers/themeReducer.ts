import { CHANGE_THEME } from '../actions'
import { AUTO_THEME_ID, AUTO_THEME_CONFIG_VALUE } from '../consts'
import config from '../config'
import { findThemeKeyByDisplayName } from '../themes'
import type { ThemeState } from '../types/redux'

const defaultTheme = (): ThemeState => {
  if (config.defaultTheme === AUTO_THEME_CONFIG_VALUE) {
    return AUTO_THEME_ID
  }
  return findThemeKeyByDisplayName(config.defaultTheme) || 'DarkTheme'
}

type ThemeAction = {
  type: string
  payload?: ThemeState
}

export const themeReducer = (
  previousState: ThemeState = defaultTheme(),
  { type, payload }: ThemeAction,
): ThemeState => {
  if (type === CHANGE_THEME && payload != null) {
    return payload
  }
  return previousState
}
