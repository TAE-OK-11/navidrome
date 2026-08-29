import { vi, type Mock } from 'vitest'
import type { UseInputValue } from 'react-admin'

export const mockFn = <T extends (...args: never[]) => unknown>(
  fn: T,
): Mock<Parameters<T>, ReturnType<T>> => vi.mocked(fn)

export const mockUseInputValue = (
  overrides: Partial<{
    value: unknown
    onChange: (value: unknown) => void
    error: string | undefined
    isTouched: boolean
  }> = {},
): UseInputValue => {
  const onChange = overrides.onChange ?? vi.fn()
  const value =
    'value' in overrides ? overrides.value : []
  return {
    id: 'test-input',
    isRequired: false,
    field: {
      name: 'libraryIds',
      value,
      onChange,
      onBlur: vi.fn(),
      ref: vi.fn(),
    },
    formState: {} as UseInputValue['formState'],
    fieldState: {
      error: overrides.error,
      invalid: Boolean(overrides.error),
      isDirty: false,
      isTouched: overrides.isTouched ?? false,
    },
  }
}
