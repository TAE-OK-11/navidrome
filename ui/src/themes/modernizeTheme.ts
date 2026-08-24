// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
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

const MUI_STATE_CLASSES = {
  checked: 'Mui-checked',
  disabled: 'Mui-disabled',
  error: 'Mui-error',
  focused: 'Mui-focused',
  focusVisible: 'Mui-focusVisible',
  required: 'Mui-required',
  expanded: 'Mui-expanded',
  selected: 'Mui-selected',
  active: 'Mui-active',
  completed: 'Mui-completed',
}

const MUI_SLOT_CLASSES = {
  notchedOutline: 'MuiOutlinedInput-notchedOutline',
  track: 'MuiSwitch-track',
  thumb: 'MuiSwitch-thumb',
  switchBase: 'MuiSwitch-switchBase',
  icon: 'MuiStepIcon-root',
}

const isPlainObject = (value) =>
  value !== null && typeof value === 'object' && !Array.isArray(value)

const rewriteLegacySelector = (selector) =>
  selector.replace(
    /(^|[^A-Za-z0-9_-])\$([A-Za-z0-9_]+)/g,
    (match, prefix, name) => {
      if (MUI_STATE_CLASSES[name]) {
        return `${prefix}.${MUI_STATE_CLASSES[name]}`
      }
      if (MUI_SLOT_CLASSES[name]) {
        return `${prefix}.${MUI_SLOT_CLASSES[name]}`
      }
      return match
    },
  )

const modernizeStyleObject = (styles) => {
  if (!isPlainObject(styles)) {
    return styles
  }

  const next = {}
  for (const [key, value] of Object.entries(styles)) {
    next[rewriteLegacySelector(key)] = isPlainObject(value)
      ? modernizeStyleObject(value)
      : value
  }
  return next
}

const modernizeStyleOverrides = (styleOverrides = {}) => {
  const next = {}
  for (const [slot, value] of Object.entries(styleOverrides)) {
    next[slot] = isPlainObject(value) ? modernizeStyleObject(value) : value
  }
  return next
}

const mergeComponentOptions = (base = {}, next = {}) => ({
  ...base,
  ...next,
  defaultProps: { ...base.defaultProps, ...next.defaultProps },
  styleOverrides: modernizeStyleOverrides({
    ...base.styleOverrides,
    ...next.styleOverrides,
  }),
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
      const nextValue =
        key === 'styleOverrides' ? modernizeStyleOverrides(value) : value
      modernComponents[component] = {
        ...current,
        [key]:
          key === 'defaultProps' || key === 'styleOverrides'
            ? { ...current[key], ...nextValue }
            : nextValue,
      }
    }
  }

  addComponentOptions(defaultProps, 'defaultProps')
  addComponentOptions(props, 'defaultProps')
  addComponentOptions(styleOverrides, 'styleOverrides')
  addComponentOptions(overrides, 'styleOverrides')

  // Ensure styleOverrides that came through merge still have modern selectors.
  for (const [component, value] of Object.entries(modernComponents)) {
    if (value?.styleOverrides) {
      modernComponents[component] = {
        ...value,
        styleOverrides: modernizeStyleOverrides(value.styleOverrides),
      }
    }
  }

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
