import { jwtDecode } from 'jwt-decode'
import { baseUrl } from './utils'
import config from './config'

// config sent from server may contain authentication info, for example when the user is authenticated
// by a reverse proxy request header
if (config.auth) {
  try {
    storeAuthenticationInfo(config.auth)
  } catch (e) {
    // eslint-disable-next-line no-console
    console.log(e)
  }
}

function storeAuthenticationInfo(authInfo) {
  authInfo.token && localStorage.setItem('token', authInfo.token)
  localStorage.setItem('userId', authInfo.id)
  localStorage.setItem('name', authInfo.name)
  localStorage.setItem('username', authInfo.username)
  authInfo.avatar && localStorage.setItem('avatar', authInfo.avatar)
  localStorage.setItem('role', authInfo.isAdmin ? 'admin' : 'regular')
  localStorage.setItem('subsonic-salt', authInfo.subsonicSalt)
  localStorage.setItem('subsonic-token', authInfo.subsonicToken)
  localStorage.setItem('is-authenticated', 'true')
}

const authProvider = {
  login: ({ username, password }) => {
    let url = baseUrl('/auth/login')
    if (config.firstTime) {
      url = baseUrl('/auth/createAdmin')
    }
    const request = new Request(url, {
      method: 'POST',
      body: JSON.stringify({ username, password }),
      headers: new Headers({ 'Content-Type': 'application/json' }),
    })
    return fetch(request)
      .then((response) => {
        if (response.status < 200 || response.status >= 300) {
          throw new Error(response.statusText)
        }
        return response.json()
      })
      .then((response) => {
        jwtDecode(response.token) // Validate token
        storeAuthenticationInfo(response)
        // Avoid "going to create admin" dialog after logout/login without a refresh
        config.firstTime = false
        return response
      })
      .catch((error) => {
        if (
          error.message === 'Failed to fetch' ||
          error.stack === 'TypeError: Failed to fetch'
        ) {
          throw new Error('errors.network_error')
        }

        throw new Error(error)
      })
  },

  logout: () => {
    removeItems()
    if (config.extAuthLogoutURL) {
      window.location.href = config.extAuthLogoutURL
      return Promise.resolve(false)
    }
    return Promise.resolve()
  },

  checkAuth: () =>
    hasValidAuthentication()
      ? Promise.resolve()
      : Promise.reject({ redirectTo: '/login' }),

  checkError: ({ status }) => {
    if (status === 401) {
      removeItems()
      return Promise.reject()
    }
    return Promise.resolve()
  },

  getPermissions: () => {
    if (!hasValidAuthentication()) {
      // Resource registration runs before react-admin performs checkAuth.
      // Resolving with no permissions lets the router finish initializing so
      // checkAuth can redirect anonymous or stale sessions to the login page.
      return Promise.resolve(null)
    }
    const role = localStorage.getItem('role')
    return Promise.resolve(role)
  },

  getIdentity: () => {
    if (!hasValidAuthentication()) {
      return Promise.reject({ redirectTo: '/login' })
    }
    return Promise.resolve({
      id: localStorage.getItem('username'),
      fullName: localStorage.getItem('name'),
      avatar: localStorage.getItem('avatar'),
    })
  },
}

const hasValidAuthentication = () => {
  const authenticated = localStorage.getItem('is-authenticated') === 'true'
  const token = localStorage.getItem('token')
  const username = localStorage.getItem('username')
  const role = localStorage.getItem('role')
  if (
    !authenticated ||
    !token ||
    !username ||
    (role !== 'admin' && role !== 'regular')
  ) {
    removeItems()
    return false
  }

  try {
    const decoded = jwtDecode(token)
    if (decoded.exp && decoded.exp * 1000 <= Date.now()) {
      removeItems()
      return false
    }
  } catch {
    removeItems()
    return false
  }

  return true
}

const removeItems = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('userId')
  localStorage.removeItem('name')
  localStorage.removeItem('username')
  localStorage.removeItem('avatar')
  localStorage.removeItem('role')
  localStorage.removeItem('subsonic-salt')
  localStorage.removeItem('subsonic-token')
  localStorage.removeItem('is-authenticated')
}

export default authProvider
