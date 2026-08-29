import React, { useState, useCallback } from 'react'
import { Field, Form } from 'react-final-form'
import type { FieldRenderProps } from 'react-final-form'
import { useDispatch } from 'react-redux'
import { useLocation } from 'react-router-dom'
import Button from '@mui/material/Button'
import Card from '@mui/material/Card'
import CardActions from '@mui/material/CardActions'
import CircularProgress from '@mui/material/CircularProgress'
import Link from '@mui/material/Link'
import TextField from '@mui/material/TextField'
import Box from '@mui/material/Box'
import {
  ThemeProvider,
  StyledEngineProvider,
  createTheme,
} from '@mui/material/styles'
import type { ThemeOptions } from '@mui/material/styles'
import { useLogin, useNotify, useTranslate } from 'react-admin'
import Logo from '../icons/android-icon-192x192.png'

import Notification from './Notification'
import useCurrentTheme from '../themes/useCurrentTheme'
import config from '../config'
import { clearQueue } from '../actions'
import { INSIGHTS_DOC_URL } from '../consts'
import modernizeTheme from '../themes/modernizeTheme'
import { componentStyleOverride } from '../themes/componentStyleOverride'

const loginSlotSx = (slot, styles) => (theme) => ({
  ...styles,
  ...componentStyleOverride(theme, 'NDLogin', slot),
})

const mainSx = loginSlotSx('main', {
  display: 'flex',
  flexDirection: 'column',
  minHeight: '100vh',
  alignItems: 'center',
  justifyContent: 'flex-start',
  background: `url(${config.loginBackgroundURL})`,
  backgroundRepeat: 'no-repeat',
  backgroundSize: 'cover',
  backgroundPosition: 'center',
})

const cardSx = loginSlotSx('card', {
  minWidth: 300,
  mt: '6em',
  overflow: 'visible',
})

const avatarSx = loginSlotSx('avatar', {
  m: '1em',
  display: 'flex',
  justifyContent: 'center',
  mt: '-3em',
})

const iconSx = loginSlotSx('icon', {
  backgroundColor: 'transparent',
  width: '6.3em',
  height: '6.3em',
})

const welcomeSx = loginSlotSx('welcome', {
  mt: '1em',
  p: '0 1em 1em',
  display: 'flex',
  justifyContent: 'center',
  flexWrap: 'wrap',
  color: '#3f51b5',
})

const renderInput = ({
  meta: { touched, error } = {},
  input: { ...inputProps },
  ...props
}: FieldRenderProps<string>) => (
  <TextField
    {...props}
    {...inputProps}
    error={!!(touched && error)}
    slotProps={{
      htmlInput: {
        // Mobile keyboards: suppress capitalization and correction for login fields.
        autoCapitalize: 'none',
        autoCorrect: 'off',
      },
    }}
    helperText={touched && error}
    fullWidth
  />
)

const FormLogin = ({ loading, handleSubmit, validate }) => {
  const translate = useTranslate()

  return (
    <Form
      onSubmit={handleSubmit}
      validate={validate}
      render={({ handleSubmit }) => (
        <form onSubmit={handleSubmit} noValidate>
          <Box sx={mainSx}>
            <Card sx={cardSx}>
              <Box sx={avatarSx}>
                <Box component="img" src={Logo} sx={iconSx} alt={'logo'} />
              </Box>
              <Box
                sx={loginSlotSx('systemName', {
                  mt: '1em',
                  display: 'flex',
                  justifyContent: 'center',
                  color: '#3f51b5',
                })}
              >
                <Box
                  component="a"
                  href="https://www.navidrome.org"
                  target="_blank"
                  rel="noopener noreferrer"
                  sx={loginSlotSx('systemNameLink', {
                    textDecoration: 'none',
                  })}
                >
                  Navidrome
                </Box>
              </Box>
              {config.welcomeMessage && (
                <Box
                  sx={welcomeSx}
                  // Use dangerouslySetInnerHTML to allow admins to configure
                  // whatever content they want
                  dangerouslySetInnerHTML={{ __html: config.welcomeMessage }}
                />
              )}
              <Box sx={loginSlotSx('form', { p: '0 1em 1em' })}>
                <Box sx={loginSlotSx('input', { mt: '1em' })}>
                  <Field
                    autoFocus
                    name="username"
                    component={renderInput}
                    label={translate('ra.auth.username')}
                    disabled={loading}
                    spellCheck={false}
                  />
                </Box>
                <Box sx={loginSlotSx('input', { mt: '1em' })}>
                  <Field
                    name="password"
                    component={renderInput}
                    label={translate('ra.auth.password')}
                    type="password"
                    disabled={loading}
                  />
                </Box>
              </Box>
              <CardActions sx={loginSlotSx('actions', { p: '0 1em 1em' })}>
                <Button
                  variant="contained"
                  type="submit"
                  color="primary"
                  disabled={loading}
                  sx={loginSlotSx('button', {})}
                  fullWidth
                >
                  {loading && <CircularProgress size={25} thickness={2} />}
                  {translate('ra.auth.sign_in')}
                </Button>
              </CardActions>
            </Card>
            <Notification />
          </Box>
        </form>
      )}
    />
  )
}

const InsightsNotice = ({ url }) => {
  const translate = useTranslate()

  const anchorRegex = /\[(.+?)]/g
  const originalMsg = translate('ra.auth.insightsCollectionNote')

  // Split the entire message on newlines
  const lines = originalMsg.split('\n')

  const renderedLines = lines.map((line, lineIndex) => {
    const segments: React.ReactNode[] = []
    let lastIndex = 0
    let match

    // Find bracketed text in each line
    while ((match = anchorRegex.exec(line)) !== null) {
      // match.index is where "[something]" starts
      // match[1] is the text inside the brackets
      const bracketText = match[1]

      // Push the text before the bracket
      segments.push(line.slice(lastIndex, match.index))

      // Push the <Link> component
      segments.push(
        <Link
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          key={`${lineIndex}-${match.index}`}
          sx={{ cursor: 'pointer' }}
        >
          {bracketText}
        </Link>,
      )

      // Update lastIndex to the character right after the bracketed text
      lastIndex = match.index + match[0].length
    }

    // Push the remaining text after the last bracket
    segments.push(line.slice(lastIndex))

    // Return this line’s parts, plus a <br/> if not the last line
    return (
      <React.Fragment key={lineIndex}>
        {segments}
        {lineIndex < lines.length - 1 && <br />}
      </React.Fragment>
    )
  })

  return (
    <Box
      sx={loginSlotSx('message', {
        mt: '1em',
        p: '0 1em 1em',
        textAlign: 'center',
        wordBreak: 'break-word',
        fontSize: '0.875em',
      })}
    >
      {renderedLines}
    </Box>
  )
}

const FormSignUp = ({ loading, handleSubmit, validate }) => {
  const translate = useTranslate()

  return (
    <Form
      onSubmit={handleSubmit}
      validate={validate}
      render={({ handleSubmit }) => (
        <form onSubmit={handleSubmit} noValidate>
          <Box sx={mainSx}>
            <Card sx={cardSx}>
              <Box sx={avatarSx}>
                <Box component="img" src={Logo} sx={iconSx} alt={'logo'} />
              </Box>
              <Box sx={welcomeSx}>{translate('ra.auth.welcome1')}</Box>
              <Box sx={welcomeSx}>{translate('ra.auth.welcome2')}</Box>
              <Box sx={loginSlotSx('form', { p: '0 1em 1em' })}>
                <Box sx={loginSlotSx('input', { mt: '1em' })}>
                  <Field
                    autoFocus
                    name="username"
                    component={renderInput}
                    label={translate('ra.auth.username')}
                    disabled={loading}
                    spellCheck={false}
                  />
                </Box>
                <Box sx={loginSlotSx('input', { mt: '1em' })}>
                  <Field
                    name="password"
                    component={renderInput}
                    label={translate('ra.auth.password')}
                    type="password"
                    disabled={loading}
                  />
                </Box>
                <Box sx={loginSlotSx('input', { mt: '1em' })}>
                  <Field
                    name="confirmPassword"
                    component={renderInput}
                    label={translate('ra.auth.confirmPassword')}
                    type="password"
                    disabled={loading}
                  />
                </Box>
              </Box>
              <CardActions sx={loginSlotSx('actions', { p: '0 1em 1em' })}>
                <Button
                  variant="contained"
                  type="submit"
                  color="primary"
                  disabled={loading}
                  sx={loginSlotSx('button', {})}
                  fullWidth
                >
                  {loading && <CircularProgress size={25} thickness={2} />}
                  {translate('ra.auth.buttonCreateAdmin')}
                </Button>
              </CardActions>
              <InsightsNotice url={INSIGHTS_DOC_URL} />
            </Card>
            <Notification />
          </Box>
        </form>
      )}
    />
  )
}

const Login = () => {
  const [loading, setLoading] = useState(false)
  const location = useLocation()
  const translate = useTranslate()
  const notify = useNotify()
  const login = useLogin()
  const dispatch = useDispatch()

  const handleSubmit = useCallback(
    (auth) => {
      setLoading(true)
      dispatch(clearQueue())
      login(auth, location.state?.nextPathname || '/').catch((error) => {
        setLoading(false)
        notify(
          typeof error === 'string'
            ? error
            : typeof error === 'undefined' || !error.message
              ? 'ra.auth.sign_in_error'
              : error.message,
          { type: 'warning' },
        )
      })
    },
    [dispatch, login, notify, setLoading, location],
  )

  const validateLogin = useCallback(
    (values: { username?: string; password?: string }) => {
      const errors: { username?: string; password?: string } = {}
      if (!values.username) {
        errors.username = translate('ra.validation.required')
      }
      if (!values.password) {
        errors.password = translate('ra.validation.required')
      }
      return errors
    },
    [translate],
  )

  const validateSignup = useCallback(
    (values: {
      username?: string
      password?: string
      confirmPassword?: string
    }) => {
      const errors = validateLogin(values) as {
        username?: string
        password?: string
        confirmPassword?: string
      }
      const regex = /^\w+$/g
      if (values.username && !values.username.match(regex)) {
        errors.username = translate('ra.validation.invalidChars')
      }
      if (!values.confirmPassword) {
        errors.confirmPassword = translate('ra.validation.required')
      }
      if (values.confirmPassword !== values.password) {
        errors.confirmPassword = translate('ra.validation.passwordDoesNotMatch')
      }
      return errors
    },
    [translate, validateLogin],
  )

  if (config.firstTime) {
    return (
      <FormSignUp
        handleSubmit={handleSubmit}
        validate={validateSignup}
        loading={loading}
      />
    )
  }
  return (
    <FormLogin
      handleSubmit={handleSubmit}
      validate={validateLogin}
      loading={loading}
    />
  )
}

// Keep the login tree under the selected theme so its sx callbacks resolve
// custom component overrides from the same theme as the rest of the app.
const LoginWithTheme = (props) => {
  const theme = useCurrentTheme()
  return (
    <StyledEngineProvider injectFirst>
      <ThemeProvider theme={createTheme(modernizeTheme(theme) as ThemeOptions)}>
        <Login {...props} />
      </ThemeProvider>
    </StyledEngineProvider>
  )
}

export default LoginWithTheme
