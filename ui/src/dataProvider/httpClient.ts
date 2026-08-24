import { fetchUtils } from 'react-admin'
import type { Options } from 'react-admin'
import { v4 as uuidv4 } from 'uuid'
import { baseUrl } from '../utils'
import config from '../config'
import { jwtDecode } from 'jwt-decode'

const customAuthorizationHeader = 'X-ND-Authorization'
export const clientUniqueIdHeader = 'X-ND-Client-Unique-Id'
export const clientUniqueId = uuidv4()

type NavidromeToken = { uid: string }

const httpClient = (url: string, options: Options = {}) => {
  url = baseUrl(url)
  const headers =
    options.headers instanceof Headers
      ? options.headers
      : new Headers({ Accept: 'application/json' })
  headers.set(clientUniqueIdHeader, clientUniqueId)
  const token = localStorage.getItem('token')
  if (token) {
    headers.set(customAuthorizationHeader, `Bearer ${token}`)
  }
  return fetchUtils.fetchJson(url, { ...options, headers }).then((response) => {
    const token = response.headers.get(customAuthorizationHeader)
    if (token) {
      const decoded = jwtDecode<NavidromeToken>(token)
      localStorage.setItem('token', token)
      localStorage.setItem('userId', decoded.uid)
      // Avoid going to create admin dialog after logout/login without a refresh
      config.firstTime = false
    }
    return response
  })
}

export default httpClient
