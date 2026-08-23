import { useTheme } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'
import { createElement, forwardRef } from 'react'

/**
 * MUI v4 withWidth replacement using useMediaQuery.
 * @see https://mui.com/material-ui/react-use-media-query/#migrating-from-withwidth
 */
export const useWidth = (options = {}) => {
  const theme = useTheme()
  // Accept both MUI (`noSsr`) and legacy withWidth (`noSSR`) spellings.
  const { noSsr, noSSR, ...mediaOptions } = options
  const queryOptions = {
    noSsr: noSsr ?? noSSR ?? true,
    ...mediaOptions,
  }

  const isXs = useMediaQuery(theme.breakpoints.up('xs'), queryOptions)
  const isSm = useMediaQuery(theme.breakpoints.up('sm'), queryOptions)
  const isMd = useMediaQuery(theme.breakpoints.up('md'), queryOptions)
  const isLg = useMediaQuery(theme.breakpoints.up('lg'), queryOptions)
  const isXl = useMediaQuery(theme.breakpoints.up('xl'), queryOptions)

  if (isXl) return 'xl'
  if (isLg) return 'lg'
  if (isMd) return 'md'
  if (isSm) return 'sm'
  if (isXs) return 'xs'
  return 'xs'
}

/**
 * Drop-in HOC compatible with the former @mui/material/withWidth API.
 */
export const withWidth =
  (options = {}) =>
  (WrappedComponent) => {
    const WithWidth = forwardRef(function WithWidth(props, ref) {
      const width = useWidth(options)
      return createElement(WrappedComponent, { width, ref, ...props })
    })
    WithWidth.displayName = `WithWidth(${
      WrappedComponent.displayName || WrappedComponent.name || 'Component'
    })`
    return WithWidth
  }

export default withWidth
