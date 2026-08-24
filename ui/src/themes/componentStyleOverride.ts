import type { Theme } from '@mui/material/styles'

type StyleOverride =
  | Record<string, unknown>
  | ((args: { theme: Theme; ownerState?: unknown }) => Record<string, unknown>)

type ThemeWithCustomComponents = Theme & {
  components?: Record<
    string,
    { styleOverrides?: Record<string, StyleOverride> }
  >
}

// Keep named Navidrome/React-admin theme overrides working while components
// move from the makeStyles compatibility layer to MUI's sx/styled APIs.
export const componentStyleOverride = (
  theme: Theme,
  component: string,
  slot: string,
  ownerState?: unknown,
) => {
  const value = (theme as ThemeWithCustomComponents).components?.[component]
    ?.styleOverrides?.[slot]
  return typeof value === 'function'
    ? value({ theme, ownerState })
    : (value ?? {})
}
