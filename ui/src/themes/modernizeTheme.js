import { createBreakpoints, createSpacing } from '@mui/system'

// Navidrome's themes still use the v4 `overrides`, `props`, and palette `type`
// shape. Convert it once without relying on MUI's removed/deprecated adapter.
const modernizeTheme = (inputTheme) => {
  const {
    components = {},
    defaultProps = {},
    mixins = {},
    overrides = {},
    palette = {},
    props = {},
    styleOverrides = {},
    ...other
  } = inputTheme
  const modernComponents = { ...components }

  const addComponentOptions = (options, key) => {
    for (const [component, value] of Object.entries(options)) {
      modernComponents[component] = {
        ...modernComponents[component],
        [key]: value,
      }
    }
  }

  addComponentOptions(defaultProps, 'defaultProps')
  addComponentOptions(props, 'defaultProps')
  addComponentOptions(styleOverrides, 'styleOverrides')
  addComponentOptions(overrides, 'styleOverrides')

  const spacing = createSpacing(inputTheme.spacing)
  const breakpoints = createBreakpoints(inputTheme.breakpoints || {})
  const { type, mode, ...paletteOptions } = palette
  const finalMode = mode || type || 'light'

  return {
    ...other,
    components: modernComponents,
    spacing,
    mixins: {
      gutters: (styles = {}) => ({
        paddingLeft: spacing(2),
        paddingRight: spacing(2),
        ...styles,
        [breakpoints.up('sm')]: {
          paddingLeft: spacing(3),
          paddingRight: spacing(3),
          ...styles[breakpoints.up('sm')],
        },
      }),
      ...mixins,
    },
    palette: {
      text: {
        hint:
          finalMode === 'dark'
            ? 'rgba(255, 255, 255, 0.5)'
            : 'rgba(0, 0, 0, 0.38)',
      },
      mode: finalMode,
      // Kept for Navidrome theme consumers until every theme definition is
      // migrated; MUI itself reads `mode`.
      type: finalMode,
      ...paletteOptions,
    },
  }
}

export default modernizeTheme
