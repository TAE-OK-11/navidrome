import { describe, expect, it } from 'vitest'
import modernizeTheme from './modernizeTheme'

describe('modernizeTheme', () => {
  it('maps legacy theme options to the supported MUI 9 shape', () => {
    const theme = modernizeTheme({
      palette: { type: 'dark', primary: { main: '#123456' } },
      props: { MuiButton: { size: 'small' } },
      overrides: { MuiButton: { root: { textTransform: 'none' } } },
      components: { MuiButton: { variants: [{ props: { compact: true } }] } },
      spacing: 10,
    })

    expect(theme.palette).toMatchObject({
      mode: 'dark',
      type: 'dark',
      primary: { main: '#123456' },
    })
    expect(theme.components.MuiButton).toMatchObject({
      defaultProps: { size: 'small' },
      styleOverrides: { root: { textTransform: 'none' } },
      variants: [{ props: { compact: true } }],
    })
    expect(theme.spacing(2)).toBe('20px')
  })

  it('rewrites legacy JSS state and slot selectors for Emotion', () => {
    const theme = modernizeTheme({
      overrides: {
        MuiSwitch: {
          colorSecondary: {
            '&$checked': { color: '#f00' },
            '&$checked + $track': { backgroundColor: '#0f0' },
          },
        },
        MuiOutlinedInput: {
          root: {
            '& $notchedOutline': { borderColor: '#111' },
            '&$focused $notchedOutline': { borderColor: '#222' },
          },
        },
      },
    })

    expect(theme.components.MuiSwitch!.styleOverrides!.colorSecondary).toEqual({
      '&.Mui-checked': { color: '#f00' },
      '&.Mui-checked + .MuiSwitch-track': { backgroundColor: '#0f0' },
    })
    expect(theme.components.MuiOutlinedInput!.styleOverrides!.root).toEqual({
      '& .MuiOutlinedInput-notchedOutline': { borderColor: '#111' },
      '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
        borderColor: '#222',
      },
    })
  })
})
