import legacyWithStyles from '@mui/styles/withStyles'
import { createTheme } from '@mui/material/styles'

const defaultTheme = createTheme()

const withStyles = (styles, options = {}) =>
  legacyWithStyles(styles, { defaultTheme, ...options })

export default withStyles
