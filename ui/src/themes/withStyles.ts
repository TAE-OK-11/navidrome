import { createElement, forwardRef } from 'react'
import makeStyles from './makeStyles'

/**
 * @mui/styles withStyles-compatible wrapper.
 * Prefer styled()/sx for new code; this exists for remaining call sites.
 */
const withStyles = (stylesOrCreator, options = {}) => {
  const useStyles = makeStyles(stylesOrCreator, options)

  return (Component) => {
    const WithStyles = forwardRef(function WithStyles(props, ref) {
      const classes = useStyles(props)
      return createElement(Component, { ...props, classes, ref })
    })

    WithStyles.displayName = `WithStyles(${
      Component.displayName || Component.name || 'Component'
    })`
    return WithStyles
  }
}

export default withStyles
