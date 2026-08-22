import { createBreakpoints, createSpacing } from '@mui/system'

const modernComponentDefaults = {
  MuiAppBar: {
    styleOverrides: {
      root: ({ theme }) => ({
        backgroundColor: theme.palette.background.paper,
        backgroundImage: 'none',
        color: theme.palette.text.primary,
        boxShadow: `0 1px 0 ${theme.palette.divider}, 0 8px 24px rgba(0, 0, 0, 0.12)`,
      }),
    },
  },
  MuiButton: {
    defaultProps: { disableElevation: true },
    styleOverrides: {
      root: {
        borderRadius: 12,
        minHeight: 40,
        textTransform: 'none',
        fontWeight: 650,
      },
    },
  },
  MuiCard: {
    styleOverrides: {
      root: ({ theme }) => ({
        border: `1px solid ${theme.palette.divider}`,
        borderRadius: 18,
        backgroundImage: 'none',
        boxShadow: '0 10px 30px rgba(0, 0, 0, 0.12)',
      }),
    },
  },
  MuiChip: {
    styleOverrides: {
      root: { borderRadius: 10, fontWeight: 600 },
    },
  },
  MuiIconButton: {
    styleOverrides: {
      root: {
        borderRadius: 12,
        transition: 'background-color 150ms ease, transform 150ms ease',
        '&:hover': { transform: 'translateY(-1px)' },
      },
    },
  },
  MuiListItemButton: {
    styleOverrides: {
      root: { borderRadius: 10, margin: '2px 8px' },
    },
  },
  MuiPaper: {
    styleOverrides: {
      root: { backgroundImage: 'none' },
      rounded: { borderRadius: 16 },
    },
  },
  MuiTableCell: {
    styleOverrides: {
      root: ({ theme }) => ({ borderBottomColor: theme.palette.divider }),
      head: { fontWeight: 700 },
    },
  },
  MuiTextField: {
    defaultProps: { variant: 'outlined' },
  },
  MuiToolbar: {
    styleOverrides: {
      root: { minHeight: 64 },
    },
  },
}

const mergeComponentOptions = (base = {}, next = {}) => ({
  ...base,
  ...next,
  defaultProps: { ...base.defaultProps, ...next.defaultProps },
  styleOverrides: { ...base.styleOverrides, ...next.styleOverrides },
})

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
  const modernComponents = { ...modernComponentDefaults }

  for (const [component, value] of Object.entries(components)) {
    modernComponents[component] = mergeComponentOptions(
      modernComponents[component],
      value,
    )
  }

  const addComponentOptions = (options, key) => {
    for (const [component, value] of Object.entries(options)) {
      const current = modernComponents[component] || {}
      modernComponents[component] = {
        ...current,
        [key]:
          key === 'defaultProps' || key === 'styleOverrides'
            ? { ...current[key], ...value }
            : value,
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
