import { forwardRef } from 'react'
import MuiCheckbox from '@mui/material/Checkbox'
import MuiSnackbar from '@mui/material/Snackbar'
import MuiTextField from '@mui/material/TextField'

// The exact @mui/material import is aliased back to this module, so reference
// the native barrel by file path to avoid a resolver cycle.
// eslint-disable-next-line react-refresh/only-export-components
export * from '../../node_modules/@mui/material/index.mjs'

const mergeSlotProp = (legacyProps, slotProp) => {
  if (!legacyProps) return slotProp
  if (typeof slotProp === 'function') {
    return (ownerState) => ({ ...legacyProps, ...slotProp(ownerState) })
  }
  return { ...legacyProps, ...slotProp }
}

// React-admin 5.15 supports MUI 9, but still supplies the removed MUI 5 props
// alongside slotProps for backwards compatibility. Strip those duplicate props
// at the package boundary so MUI 9 never forwards them to the DOM.
export const TextField = forwardRef(function TextField(
  {
    FormHelperTextProps,
    InputLabelProps,
    InputProps,
    SelectProps,
    inputProps,
    slotProps = {},
    ...props
  },
  ref,
) {
  return (
    <MuiTextField
      {...props}
      ref={ref}
      slotProps={{
        ...slotProps,
        formHelperText: mergeSlotProp(
          FormHelperTextProps,
          slotProps.formHelperText,
        ),
        htmlInput: mergeSlotProp(inputProps, slotProps.htmlInput),
        input: mergeSlotProp(InputProps, slotProps.input),
        inputLabel: mergeSlotProp(InputLabelProps, slotProps.inputLabel),
        select: mergeSlotProp(SelectProps, slotProps.select),
      }}
    />
  )
})

export const Snackbar = forwardRef(function Snackbar(
  { ContentProps, TransitionProps, slotProps = {}, ...props },
  ref,
) {
  return (
    <MuiSnackbar
      {...props}
      ref={ref}
      slotProps={{
        ...slotProps,
        content: mergeSlotProp(ContentProps, slotProps.content),
        transition: mergeSlotProp(TransitionProps, slotProps.transition),
      }}
    />
  )
})

export const Checkbox = forwardRef(function Checkbox(
  { inputProps, inputRef, slotProps = {}, ...props },
  ref,
) {
  return (
    <MuiCheckbox
      {...props}
      ref={ref}
      slotProps={{
        ...slotProps,
        input: mergeSlotProp({ ...inputProps, ref: inputRef }, slotProps.input),
      }}
    />
  )
})
