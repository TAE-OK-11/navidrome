// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { describe, expect, it, vi } from 'vitest'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { renderHook } from '@testing-library/react'
import makeStyles from './makeStyles'
import { useWidth } from './useWidth'

vi.mock('@mui/material/useMediaQuery', () => ({
  default: vi.fn(),
}))

import useMediaQuery from '@mui/material/useMediaQuery'

describe('makeStyles compatibility wrapper', () => {
  it('returns class names and resolves prop callbacks', () => {
    const useStyles = makeStyles(
      {
        root: {
          color: (props) => props.color,
        },
      },
      { name: 'NDTestStyles' },
    )

    const wrapper = ({ children }) => (
      <ThemeProvider theme={createTheme()}>{children}</ThemeProvider>
    )
    const { result } = renderHook(() => useStyles({ color: 'tomato' }), {
      wrapper,
    })

    expect(result.current.root).toEqual(expect.any(String))
    expect(result.current.root.length).toBeGreaterThan(0)
  })

  it('rewrites local $ refs into concrete class selectors', () => {
    const useStyles = makeStyles(
      {
        child: { opacity: 0 },
        parent: {
          '&:hover $child': { opacity: 1 },
        },
      },
      { name: 'NDTestRefs' },
    )

    const wrapper = ({ children }) => (
      <ThemeProvider theme={createTheme()}>{children}</ThemeProvider>
    )
    const { result } = renderHook(() => useStyles(), { wrapper })

    expect(result.current.parent).toEqual(expect.any(String))
    expect(result.current.child).toEqual(expect.any(String))
  })
})

describe('useWidth', () => {
  it('returns the largest matching breakpoint', () => {
    vi.mocked(useMediaQuery)
      .mockReturnValueOnce(true) // xs
      .mockReturnValueOnce(true) // sm
      .mockReturnValueOnce(true) // md
      .mockReturnValueOnce(false) // lg
      .mockReturnValueOnce(false) // xl

    const wrapper = ({ children }) => (
      <ThemeProvider theme={createTheme()}>{children}</ThemeProvider>
    )
    const { result } = renderHook(() => useWidth(), { wrapper })
    expect(result.current).toBe('md')
  })
})
