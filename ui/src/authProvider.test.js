import { beforeEach, describe, expect, it } from 'vitest'
import authProvider from './authProvider'

const token = (payload) => {
  const encode = (value) =>
    btoa(JSON.stringify(value))
      .replaceAll('+', '-')
      .replaceAll('/', '_')
      .replaceAll('=', '')
  return `${encode({ alg: 'none', typ: 'JWT' })}.${encode(payload)}.`
}

const storeValidSession = (overrides = {}) => {
  localStorage.setItem('is-authenticated', 'true')
  localStorage.setItem(
    'token',
    token({ exp: Math.floor(Date.now() / 1000) + 3600 }),
  )
  localStorage.setItem('username', 'listener')
  localStorage.setItem('name', 'Listener')
  localStorage.setItem('role', 'regular')
  Object.entries(overrides).forEach(([key, value]) => {
    if (value === null) {
      localStorage.removeItem(key)
    } else {
      localStorage.setItem(key, value)
    }
  })
}

describe('authProvider session recovery', () => {
  beforeEach(() => localStorage.clear())

  it('accepts a complete unexpired session', async () => {
    storeValidSession()

    await expect(authProvider.checkAuth()).resolves.toBeUndefined()
    await expect(authProvider.getPermissions()).resolves.toBe('regular')
    await expect(authProvider.getIdentity()).resolves.toMatchObject({
      id: 'listener',
      fullName: 'Listener',
    })
  })

  it('clears and rejects an incomplete persisted session', async () => {
    storeValidSession({ token: null })

    await expect(authProvider.checkAuth()).rejects.toEqual({
      redirectTo: '/login',
    })
    expect(localStorage.getItem('is-authenticated')).toBeNull()
  })

  it('clears and rejects an expired token', async () => {
    storeValidSession({
      token: token({ exp: Math.floor(Date.now() / 1000) - 60 }),
    })

    await expect(authProvider.getPermissions()).rejects.toEqual({
      redirectTo: '/login',
    })
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('clears and rejects a malformed token', async () => {
    storeValidSession({ token: 'not-a-jwt' })

    await expect(authProvider.getIdentity()).rejects.toEqual({
      redirectTo: '/login',
    })
    expect(localStorage.getItem('role')).toBeNull()
  })
})
