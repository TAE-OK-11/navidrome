import legacyMakeStyles from '@mui/styles/makeStyles'
import { createTheme } from '@mui/material/styles'

// @mui/styles is frozen on its v6 theme context, while the application now
// uses MUI 9. Supplying a real MUI theme keeps isolated components (including
// tests and the public share player) functional when no legacy provider is
// present. The application-level provider still overrides this fallback with
// the selected Navidrome theme.
const defaultTheme = createTheme()

const makeStyles = (styles, options = {}) =>
  legacyMakeStyles(styles, { defaultTheme, ...options })

export default makeStyles
