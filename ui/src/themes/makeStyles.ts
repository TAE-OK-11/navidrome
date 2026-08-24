// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { makeStyles as tssMakeStyles } from 'tss-react/mui-compat'

const isPlainObject = (value) =>
  value !== null && typeof value === 'object' && !Array.isArray(value)

// Resolve @mui/styles-style property callbacks: { color: (props) => props.color }
const resolvePropCallbacks = (styles, props) => {
  if (!isPlainObject(styles)) {
    return typeof styles === 'function' ? styles(props) : styles
  }

  const resolved = {}
  for (const [key, value] of Object.entries(styles)) {
    if (typeof value === 'function') {
      resolved[key] = value(props)
    } else if (isPlainObject(value)) {
      resolved[key] = resolvePropCallbacks(value, props)
    } else {
      resolved[key] = value
    }
  }
  return resolved
}

// Convert JSS local refs (`$ruleName`) to Emotion class selectors.
const rewriteDollarRefs = (styles, classes) => {
  if (!isPlainObject(styles)) {
    return styles
  }

  const rewritten = {}
  for (const [key, value] of Object.entries(styles)) {
    const nextKey = key.replace(
      /(^|[^A-Za-z0-9_-])\$([A-Za-z0-9_]+)/g,
      (_, prefix, ruleName) => `${prefix}.${classes[ruleName]}`,
    )
    rewritten[nextKey] = isPlainObject(value)
      ? rewriteDollarRefs(value, classes)
      : value
  }
  return rewritten
}

/**
 * @mui/styles makeStyles-compatible wrapper on top of tss-react.
 * Keeps the call shape `makeStyles(styles, options)(props) => classes`.
 */
const makeStyles = (stylesOrCreator, options = {}) => {
  const name =
    typeof options.name === 'string'
      ? options.name
      : options.name && typeof options.name === 'object'
        ? Object.keys(options.name)[0]
        : undefined

  const useTssStyles = tssMakeStyles({ name })((theme, params, classes) => {
    const raw =
      typeof stylesOrCreator === 'function'
        ? stylesOrCreator(theme)
        : stylesOrCreator
    const withProps = resolvePropCallbacks(raw, params || {})
    return rewriteDollarRefs(withProps, classes)
  })

  return function useStyles(props) {
    const { classes } = useTssStyles(props)
    return classes
  }
}

export default makeStyles
