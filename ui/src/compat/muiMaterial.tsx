import { forwardRef, type InputHTMLAttributes, type Ref } from 'react'
import MuiCheckbox from '@mui/material/Checkbox'
import MuiSnackbar from '@mui/material/Snackbar'
import MuiTextField from '@mui/material/TextField'
import type { CheckboxProps as MuiCheckboxProps } from '@mui/material/Checkbox'
import type { SnackbarProps as MuiSnackbarProps } from '@mui/material/Snackbar'
import type { TextFieldProps as MuiTextFieldProps } from '@mui/material/TextField'
import type { FormHelperTextProps } from '@mui/material/FormHelperText'
import type { InputLabelProps } from '@mui/material/InputLabel'
import type { InputProps } from '@mui/material/Input'
import type { SelectProps } from '@mui/material/Select'
import type { SnackbarContentProps } from '@mui/material/SnackbarContent'
import type { TransitionProps } from '@mui/material/transitions'

// The exact @mui/material import is aliased back to this module, so reference
// the native barrel by file path to avoid a resolver cycle.
// eslint-disable-next-line react-refresh/only-export-components
export * from '../../node_modules/@mui/material/index.mjs'

type SlotPropValue = unknown

const mergeSlotProp = (
  legacyProps:
    Record<string, unknown> | InputHTMLAttributes<HTMLInputElement> | undefined,
  slotProp: SlotPropValue,
): SlotPropValue => {
  if (!legacyProps) return slotProp
  if (typeof slotProp === 'function') {
    return (ownerState: unknown) => ({
      ...legacyProps,
      ...(slotProp as (ownerState: unknown) => Record<string, unknown>)(
        ownerState,
      ),
    })
  }
  if (slotProp && typeof slotProp === 'object') {
    return { ...legacyProps, ...(slotProp as Record<string, unknown>) }
  }
  return { ...legacyProps }
}

type LegacyTextFieldProps = {
  FormHelperTextProps?: Partial<FormHelperTextProps>
  InputLabelProps?: Partial<InputLabelProps>
  InputProps?: Partial<InputProps>
  SelectProps?: Partial<SelectProps>
  inputProps?: InputHTMLAttributes<HTMLInputElement>
}

type TextFieldProps = MuiTextFieldProps & LegacyTextFieldProps

type LegacySnackbarProps = {
  ContentProps?: Partial<SnackbarContentProps>
  TransitionProps?: Partial<TransitionProps>
}

type SnackbarProps = MuiSnackbarProps & LegacySnackbarProps

type LegacyCheckboxProps = {
  inputProps?: InputHTMLAttributes<HTMLInputElement>
  inputRef?: Ref<HTMLInputElement>
}

type CheckboxProps = Omit<MuiCheckboxProps, 'slotProps'> &
  LegacyCheckboxProps & {
    slotProps?: MuiCheckboxProps['slotProps']
  }

// React-admin 5.15 supports MUI 9, but still supplies the removed MUI 5 props
// alongside slotProps for backwards compatibility. Strip those duplicate props
// at the package boundary so MUI 9 never forwards them to the DOM.
export const TextField = forwardRef<HTMLDivElement, TextFieldProps>(
  function TextField(
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
        slotProps={
          {
            ...slotProps,
            formHelperText: mergeSlotProp(
              FormHelperTextProps,
              slotProps.formHelperText,
            ),
            htmlInput: mergeSlotProp(inputProps, slotProps.htmlInput),
            input: mergeSlotProp(InputProps, slotProps.input),
            inputLabel: mergeSlotProp(InputLabelProps, slotProps.inputLabel),
            select: mergeSlotProp(SelectProps, slotProps.select),
          } as MuiTextFieldProps['slotProps']
        }
      />
    )
  },
)

export const Snackbar = forwardRef<HTMLDivElement, SnackbarProps>(
  function Snackbar(
    { ContentProps, TransitionProps, slotProps = {}, ...props },
    ref,
  ) {
    return (
      <MuiSnackbar
        {...props}
        ref={ref}
        slotProps={
          {
            ...slotProps,
            content: mergeSlotProp(ContentProps, slotProps.content),
            transition: mergeSlotProp(TransitionProps, slotProps.transition),
          } as MuiSnackbarProps['slotProps']
        }
      />
    )
  },
)

export const Checkbox = forwardRef<HTMLButtonElement, CheckboxProps>(
  function Checkbox({ inputProps, inputRef, slotProps = {}, ...props }, ref) {
    return (
      <MuiCheckbox
        {...props}
        ref={ref}
        slotProps={
          {
            ...slotProps,
            input: mergeSlotProp(
              { ...inputProps, ref: inputRef },
              slotProps.input,
            ),
          } as CheckboxProps['slotProps']
        }
      />
    )
  },
)
